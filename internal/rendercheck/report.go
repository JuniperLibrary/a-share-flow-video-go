package rendercheck

import (
	"fmt"
	"io"
	"strings"
)

// WriteText 输出人类可读的体检报告。格式稳定，便于贴进 issue 或 CI 日志。
func (r Report) WriteText(w io.Writer) error {
	status := "[PASS]"
	if !r.OK {
		status = "[FAIL]"
	}
	m := r.Metrics
	if _, err := fmt.Fprintf(w, "%s %s  [%s]\n", status, r.Path, r.Format); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w,
		"  时长 %.1fs | 音频 %.1fs | 差 %.0fms | %dx%d@%.0f | 首帧亮度 %.0f / 末帧 %.0f\n",
		m.DurationSec, m.AudioSec, m.AVDiffSec*1000, m.Width, m.Height, m.FPS,
		m.FirstLuma, m.LastLuma); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w,
		"  静音 %d 段 / %.1fs（%.1f%%）| 最长 %.2fs | 黑屏 %d 段 / %.1fs\n",
		m.SilenceCount, m.SilenceTotalSec, m.SilenceRatio*100, m.LongestSilenceSec,
		m.BlankCount, m.BlankTotalSec); err != nil {
		return err
	}
	if len(r.Violations) > 0 {
		if _, err := fmt.Fprintln(w, "  违规:"); err != nil {
			return err
		}
		for _, v := range r.Violations {
			if _, err := fmt.Fprintf(w, "    - [%s] %s\n", v.Rule, v.Message); err != nil {
				return err
			}
		}
	}
	if len(r.Silences) > 0 {
		parts := make([]string, 0, len(r.Silences))
		for _, s := range r.Silences {
			parts = append(parts, fmt.Sprintf("%.2f-%.2f", s.Start, s.End))
		}
		if _, err := fmt.Fprintf(w, "  静音段: %s\n", strings.Join(parts, ", ")); err != nil {
			return err
		}
	}
	if len(r.Blanks) > 0 {
		parts := make([]string, 0, len(r.Blanks))
		for _, b := range r.Blanks {
			parts = append(parts, fmt.Sprintf("%.2f-%.2f", b.Start, b.End))
		}
		if _, err := fmt.Fprintf(w, "  黑屏段: %s\n", strings.Join(parts, ", ")); err != nil {
			return err
		}
	}
	return nil
}

// WriteSummary 输出多份报告的汇总，返回失败数。
func WriteSummary(w io.Writer, reports []Report) int {
	failed := 0
	for _, r := range reports {
		if !r.OK {
			failed++
		}
	}
	fmt.Fprintf(w, "\n汇总: %d 期通过 / %d 期失败 / 共 %d 期\n",
		len(reports)-failed, failed, len(reports))
	return failed
}
