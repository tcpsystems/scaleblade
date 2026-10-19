package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

func (a *App) printJSON(v any) error {
	enc := json.NewEncoder(a.Out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// printRaw pretty-prints an already-encoded JSON payload. If it is not valid
// JSON, the bytes are written unchanged.
func (a *App) printRaw(raw []byte) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		fmt.Fprintln(a.Out, "{}")
		return nil
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		_, werr := a.Out.Write(raw)
		return werr
	}
	buf.WriteByte('\n')
	_, err := a.Out.Write(buf.Bytes())
	return err
}

func writeTable(w io.Writer, headers []string, rows [][]string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, strings.Join(headers, "\t")); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := fmt.Fprintln(tw, strings.Join(row, "\t")); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func writeKV(w io.Writer, pairs [][2]string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, p := range pairs {
		if _, err := fmt.Fprintf(tw, "%s:\t%s\n", p[0], p[1]); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func pence(v int64) string {
	sign := ""
	if v < 0 {
		sign = "-"
		v = -v
	}
	return fmt.Sprintf("%s%d.%02d", sign, v/100, v%100)
}

func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func decodeData(raw json.RawMessage, out any) error {
	if len(raw) == 0 {
		return fmt.Errorf("response contained no data")
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode response data: %w", err)
	}
	return nil
}
