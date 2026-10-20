# Recovery runbook

How to tell whether the Scaleblade VM, the portal session and the WireGuard
egress path are healthy, and what to do in the order that avoids making things
worse. Written for an operator or an agent with SSH access to `edge`.

**Verified:** 2026-10-20 20:04–21:05Z from `edge`, plus read-only SSH probes to
the host. Anything not observed is marked **unverified**.

## What this host is

`scaleblade` (`194.180.36.11`) is the massprox **WireGuard egress node**. It is
not a workload host: no product service runs on it, and it is not a tailnet
member. Its entire state is:

| Item | Path / value | Persisted by |
| --- | --- | --- |
| WireGuard interface | `10.99.0.2/24`, UDP 51820 | `/etc/wireguard/wg0.conf`, `wg-quick@wg0.service` (enabled) |
| Forwarding | `net.ipv4.ip_forward=1` | `/etc/sysctl.d/99-wg-egress.conf` and `PostUp` |
| NAT | `MASQUERADE 10.99.0.0/24 → eth0` | `wg0.conf` `PostUp`, removed on `PostDown` |
| Firewall | `INPUT DROP`; allow lo, established, tcp/22, udp/51820, icmp | `iptables` rules, not a unit |
| SSH | key-only, `passwordauthentication no`, `permitrootlogin yes` | `/etc/ssh/sshd_config` |

The consequence of losing any of these is that **massprox stops egressing
through Neterra** and either fails or silently leaks from a Hetzner IP, which
is the exact failure the product exists to avoid.

## Control surfaces

| Surface | Command | Fails when |
| --- | --- | --- |
| Portal API | `scaleblade …` (`~/Projects/tcpsystems/scaleblade/bin/scaleblade`) | browser session expires — the common case |
| SSH | `ssh scaleblade` | host down, key missing, or provider rebuilt it |
| Tunnel (far side) | `ssh dsh 'sudo wg show wg0'` | peer down or endpoint changed |
| Egress truth | `ssh dsh 'sudo -u massprox curl -s https://ifconfig.me'` | tunnel or NAT broken |

The binary is **not installed in `PATH`**; run it from the repository, or
`make build` again after a pull. `go install` works too (see README).

## Ladder

Run the steps in order and stop at the first failure. Every step is read-only.

### 1. Is the tunnel carrying traffic?

```bash
ssh dsh 'systemctl is-active mass-proxy-gather rx-search-gateway; sudo -n /usr/local/bin/massprox-egress-gate; echo gate=$?'
ssh dsh 'sudo -u massprox curl -sS -4 https://ifconfig.me; echo'
```

Expected: both units `active`, `gate=0`, and `194.180.36.11`. A gate failure is
a refusal by design — the units are meant to be skipped, not run anyway.

If the egress address is `116.202.107.22` (dsh itself), the uid rule or the
tunnel is broken: **stop massprox before diagnosing**, or traffic leaves from a
Hetzner IP.

### 2. Is the portal session still valid?

```bash
cd ~/Projects/tcpsystems/scaleblade
./bin/scaleblade auth check        # 0 = ok, 3 = refresh needed
./bin/scaleblade auth me
```

`auth check` prints the role, the session expiry and the remaining time.
`cookies.json` is a browser session, not an API key: it dies without warning
and no command can renew it. Exit 3 is not a bug to retry.

**Refresh (the only step that needs a human):**

1. Sign in at `https://my.scaleblade.com`.
2. DevTools → Application/Storage → Cookies for `my.scaleblade.com`.
3. Copy `cf_clearance` and the portal session cookie into
   `~/.config/scaleblade/cookies.json` in the shape shown in README.md.
4. `chmod 600 ~/.config/scaleblade/cookies.json` — the client refuses any file
   with group or world bits.
5. Re-run `./bin/scaleblade auth check`.

Never paste the values into a shell argument, a log, an issue or a commit.

### 3. Is a provider task stalled?

```bash
./bin/scaleblade tasks --stuck 20m     # 4 = at least one task stopped moving
./bin/scaleblade vms deployment <vm-id> --project <project-id> --json
```

Exit 4 means a task has not updated inside the threshold. That is a provider
condition, not a local one: the portal's task agent is the only thing that can
advance it, and **cancelling requires an administrator role** — the ordinary
portal role gets `403 Administrator access is required` (observed
2026-10-19 on `TASK-01A15551-FCDE`).

Escalation is a support ticket. Include the display id, the state, the
percentage and the last `updated_at` from the two commands above.

### 4. Is the host reachable?

```bash
ssh -o BatchMode=yes scaleblade 'hostname -s; uptime; sudo -n wg show wg0'
```

Expect `scaleblade`, a Debian 13 kernel, a recent handshake and a
`latest handshake` under ~3 minutes. If SSH refuses, the provider console is
the only remaining channel, and only the provider can restore it.

### 5. Rebuild

Only if the host is gone. A reinstall **loses the primary IP only if the VM is
deleted**; reinstalling in place keeps it. Rebuild order:

1. Reinstall Debian 13 from the portal (record the task id first).
2. Install the fleet key (`~/.ssh/shared`) before anything else.
3. Restore `/etc/wireguard/wg0.conf`, `/etc/sysctl.d/99-wg-egress.conf` and the
   iptables rules from the newest fleet snapshot (see below).
4. `systemctl enable --now wg-quick@wg0`.
5. Verify from `dsh` with step 1, then re-enable the massprox units.

The WireGuard private key is in the snapshot. Restore it, never regenerate it:
the peer on `dsh` is pinned to the public key.

## Backups

`fleet-backup.service` on `core` (nightly, 7-day rsync hardlink snapshots under
`/home/kivox/Backups/fleet-auto/snapshots/`) includes scaleblade's `/etc` and
`/home/ubuntu`. The manifest records a per-host result and the script refuses
to sync when `hostname -s` does not match the label, so a renamed host cannot
be stored under the wrong name.

```bash
ssh core 'ls -1dt /home/kivox/Backups/fleet-auto/snapshots/*/ | head -3'
ssh core 'grep -i scaleblade /home/kivox/Backups/fleet-auto/snapshots/<snap>/MANIFEST.txt'
```

The nightly run on 2026-10-20 03:43Z failed with stale peer addresses; the
script's host map was corrected and a full run at 20:58Z finished `rc=0` with
`scaleblade … ok`. Check the manifest, not the exit code alone.

## Known gaps (not automated as of 2026-10-20)

- **Cookie expiry is silent.** No timer calls `auth check`; the fleet smoke
  reaches scaleblade over SSH and never notices the API is unusable.
- **Scaleblade is unmonitored.** VictoriaMetrics on `dsh` scrapes only local
  targets (`loki`, itself, blackbox probes of local services). No
  `node_exporter`, no tunnel-handshake alert.
- **The dsh egress gate is start-only.** `ExecCondition` runs at unit start; a
  tunnel that dies afterwards is not stopped by a watchdog.
- **agetty on tty1 logs `failed to get terminal attributes` every ~10s** on the
  provider's console. Cosmetic, but it fills the journal; do not disable the
  console services remotely without a provider-console fallback.
- **`fleet-backup.service` can fail silently in the journal** — it writes to
  `backup.log` and its unit failure is easy to miss.

## Do not

- Do not delete or recreate the VM to clear a stuck task: it loses the primary
  IP and the backup lineage.
- Do not retry a reinstall the provider is already running; a retry returned
  `HTTP 500` with no new task on 2026-10-19.
- Do not run `mass-proxy-gather` while the egress gate fails.
- Do not commit, log or paste cookie values, the WireGuard private key, or the
  contents of `~/.config/scaleblade/credentials.json`.
