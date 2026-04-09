package main

const (
	expPerPlacement int64 = 10
	baseLevelExp    int64 = 100
	levelExpStep    int64 = 25
)

type ProgressionSnapshot struct {
	Level            int
	TotalExp         int64
	LevelExpCurrent  int64
	LevelExpRequired int64
}

func ComputeProgressionFromPlacements(totalPlacements int64) ProgressionSnapshot {
	if totalPlacements < 0 {
		totalPlacements = 0
	}

	totalExp := totalPlacements * expPerPlacement
	level := 1
	required := baseLevelExp
	remaining := totalExp

	for remaining >= required {
		remaining -= required
		level++
		required = baseLevelExp + int64(level-1)*levelExpStep
	}

	return ProgressionSnapshot{
		Level:            level,
		TotalExp:         totalExp,
		LevelExpCurrent:  remaining,
		LevelExpRequired: required,
	}
}
