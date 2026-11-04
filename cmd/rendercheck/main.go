// Command rendercheck 体检成品视频是否满足渲染链路的不变量。
//
// 用法:
//
//	go run ./cmd/rendercheck output/2026-08-14
//	go run ./cmd/rendercheck -json output
//	go run ./cmd/rendercheck -narration 12 output/2026-08-14/全天_tick_mobile.mp4
//
// 退出码: 0 全部通过, 1 存在违规, 2 执行失败。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/a-share-flow-video-go/internal/rendercheck"
)

func main() {
	os.Exit(run())
}

func run() int {
	var (
		asJSON       = flag.Bool("json", false, "以 JSON 输出报告")
		narration    = flag.Int("narration", 0, "口播清单段数，>0 时校验缺段")
		silenceRatio = flag.Float64("silence-ratio", 0, "静音占比上限，0 表示用默认值")
		silenceGap   = flag.Float64("silence-gap", 0, "单段静音上限（秒），0 表示用默认值")
	)
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(),
			"用法: rendercheck [flags] <视频或目录>...\n默认体检 output/ 目录。\n\nflags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	paths := flag.Args()
	if len(paths) == 0 {
		paths = []string{"output"}
	}

	files, err := rendercheck.CollectVideos(paths)
	if err != nil {
		fmt.Fprintf(os.Stderr, "收集视频失败: %v\n", err)
		return 2
	}
	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "没有找到待体检的 mp4: %v\n", paths)
		return 2
	}

	reports := make([]rendercheck.Report, 0, len(files))
	for _, f := range files {
		report, err := check(f, *narration, *silenceRatio, *silenceGap)
		if err != nil {
			fmt.Fprintf(os.Stderr, "体检失败 %s: %v\n", f, err)
			return 2
		}
		reports = append(reports, report)
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(reports); err != nil {
			fmt.Fprintf(os.Stderr, "序列化报告失败: %v\n", err)
			return 2
		}
	} else {
		for _, r := range reports {
			_ = r.WriteText(os.Stdout)
		}
	}

	failed := rendercheck.WriteSummary(os.Stdout, reports)
	if failed > 0 {
		return 1
	}
	return 0
}

func check(path string, narration int, silenceRatio, silenceGap float64) (rendercheck.Report, error) {
	media, err := rendercheck.ProbeFile(path)
	if err != nil {
		return rendercheck.Report{}, err
	}
	spec := rendercheck.SpecFor(path, media)
	if narration > 0 {
		spec.NarrationSegments = narration
	}
	if silenceRatio > 0 {
		spec.MaxSilenceRatio = silenceRatio
	}
	if silenceGap > 0 {
		spec.MaxSilenceGapSec = silenceGap
	}
	return rendercheck.CheckWithSpec(path, media, spec)
}
