package tick

import "testing"

func TestPointsToSectors_PreservesOrderStructureFields(t *testing.T) {
	points := []TickPoint{
		{Time: "09:30", Name: "国产芯片", Net: 100, SuperNet: 80, BigNet: 10, ChangePct: 1.2},
		{Time: "10:00", Name: "国产芯片", Net: 253, SuperNet: 238, BigNet: 15, ChangePct: 2.5},
		{Time: "10:00", Name: "半导体", Net: 163, SuperNet: 50, BigNet: 30, ChangePct: 1.0},
	}

	sectors := PointsToSectors(points)
	if len(sectors) != 2 {
		t.Fatalf("expected 2 sectors, got %d", len(sectors))
	}

	byName := make(map[string]struct {
		Net, SuperNet, BigNet, ChangePct float64
	})
	for _, s := range sectors {
		byName[s.Name] = struct {
			Net, SuperNet, BigNet, ChangePct float64
		}{s.Net, s.SuperNet, s.BigNet, s.ChangePct}
	}

	chip := byName["国产芯片"]
	if chip.Net != 253 || chip.SuperNet != 238 || chip.BigNet != 15 || chip.ChangePct != 2.5 {
		t.Fatalf("国产芯片 latest snapshot wrong: net=%v super=%v big=%v chg=%v", chip.Net, chip.SuperNet, chip.BigNet, chip.ChangePct)
	}

	semi := byName["半导体"]
	if semi.Net != 163 || semi.SuperNet != 50 {
		t.Fatalf("半导体 snapshot wrong: net=%v super=%v", semi.Net, semi.SuperNet)
	}
}
