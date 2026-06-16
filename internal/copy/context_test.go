package copy

import (
	"strings"
	"testing"

	"github.com/a-share-flow-video-go/internal/fetcher"
)

func TestBuildStructureBlock_LeadingSuperNetDespiteFlatMarket(t *testing.T) {
	sectors := []fetcher.Sector{
		{Name: "国产芯片", Net: 253, SuperNet: 238, BigNet: 15, ChangePct: 2.5},
		{Name: "CPO概念", Net: 204, SuperNet: 180, BigNet: 24, ChangePct: 3.1},
		{Name: "半导体", Net: 163, SuperNet: 140, BigNet: 23, ChangePct: 1.8},
		{Name: "银行", Net: -300, SuperNet: -280, BigNet: -20, ChangePct: -0.5},
		{Name: "白酒", Net: -200, SuperNet: -278, BigNet: -22, ChangePct: -1.2},
	}

	inflows, _, netTotal, totalSuper, totalBig := splitSectorFlows(sectors)
	block := buildStructureBlock(netTotal, totalSuper, totalBig, inflows, sectors)

	if !strings.Contains(block, "国产芯片") || !strings.Contains(block, "超大单+238") {
		t.Fatalf("expected leading sector super net in block, got:\n%s", block)
	}
	if !strings.Contains(block, "勿写机构缺席") {
		t.Fatalf("expected hedge warning when market super net is flat, got:\n%s", block)
	}
}

func TestDeriveAngles_TechCluster(t *testing.T) {
	inflows := []sectorFlow{
		{Name: "国产芯片", Net: 253, SuperNet: 238},
		{Name: "CPO概念", Net: 204, SuperNet: 180},
		{Name: "半导体", Net: 163, SuperNet: 140},
	}
	angles := deriveAngles(inflows, nil, 620, 558)
	found := false
	for _, a := range angles {
		if strings.Contains(a, "科技链共振") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected tech cluster angle, got: %v", angles)
	}
}

func TestBuildStructureBlock_MissingOrderFields(t *testing.T) {
	sectors := []fetcher.Sector{
		{Name: "国产芯片", Net: 253},
		{Name: "半导体", Net: 163},
	}
	inflows, _, netTotal, totalSuper, totalBig := splitSectorFlows(sectors)
	block := buildStructureBlock(netTotal, totalSuper, totalBig, inflows, sectors)

	if !strings.Contains(block, "勿推断") {
		t.Fatalf("expected missing-field guardrail, got:\n%s", block)
	}
}

func TestFormatTopFlows_IncludesSuperBig(t *testing.T) {
	line := formatTopFlows([]sectorFlow{
		{Name: "国产芯片", Net: 253, SuperNet: 238, BigNet: 15, ChgPct: 2.5},
	}, true, 1)

	if !strings.Contains(line, "超大单+238") || !strings.Contains(line, "大单+15") {
		t.Fatalf("unexpected top flow line: %s", line)
	}
}
