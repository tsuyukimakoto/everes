// Package migration はTask JSONのschema versionを最新へ引き上げる。
//
// Readerは古いversionを読めること、Writerは常に最新versionを書くこと。
// この2つを守るため、読み込み経路に必ずMigrateを挟む。
package migration

import "fmt"

// Latest はこの実装が書き出すschema version。
const Latest = 2

// Step は1つのschema versionから次への変換。
type Step struct {
	From  int
	To    int
	Apply func(doc map[string]any) error
}

// steps はschema versionの昇順に並べる。
var steps = []Step{
	{From: 1, To: 2, Apply: v1ToV2},
}

// v1ToV2 はAgent Sessionごとの状態管理に対応する。
// 既存Taskのsession statusは実行状態から安全に推測できないため追加しない。
func v1ToV2(_ map[string]any) error { return nil }

// Migrate はdocを最新のschema versionへ変換する。
func Migrate(doc map[string]any) (map[string]any, error) {
	v, err := versionOf(doc)
	if err != nil {
		return nil, err
	}

	if v > Latest {
		return nil, fmt.Errorf(
			"schema_version %d is newer than supported version %d; update everes", v, Latest)
	}

	for v < Latest {
		step, ok := findStep(v)
		if !ok {
			return nil, fmt.Errorf("no migration path from schema_version %d to %d", v, Latest)
		}
		if err := step.Apply(doc); err != nil {
			return nil, fmt.Errorf("migrate v%d to v%d: %w", step.From, step.To, err)
		}
		v = step.To
		doc["schema_version"] = float64(v)
	}

	return doc, nil
}

func versionOf(doc map[string]any) (int, error) {
	raw, ok := doc["schema_version"]
	if !ok {
		return 0, fmt.Errorf("schema_version is missing")
	}
	// encoding/jsonはnumberをfloat64で読む。
	f, ok := raw.(float64)
	if !ok {
		return 0, fmt.Errorf("schema_version has unexpected type %T", raw)
	}
	v := int(f)
	if v <= 0 {
		return 0, fmt.Errorf("invalid schema_version %d", v)
	}
	return v, nil
}

func findStep(from int) (Step, bool) {
	for _, s := range steps {
		if s.From == from {
			return s, true
		}
	}
	return Step{}, false
}
