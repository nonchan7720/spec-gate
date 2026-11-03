package adapter

import (
	"reflect"
	"testing"
)

// figmaTree は Figma nodes API の document 部分の形（必要なキーだけ）。
func figmaTree() map[string]any {
	return map[string]any{
		"type": "FRAME", "name": "支払一覧",
		"children": []any{
			map[string]any{"type": "TEXT", "characters": "支払一覧"},
			map[string]any{
				"type": "COMPONENT_SET", "name": "PaymentList",
				"children": []any{
					map[string]any{"type": "COMPONENT", "name": "Default", "children": []any{
						map[string]any{"type": "TEXT", "characters": "合計"},
					}},
					map[string]any{"type": "COMPONENT", "name": "Empty"},
					"壊れた子は無視",
				},
			},
			map[string]any{"type": "TEXT"}, // characters なし
		},
	}
}

func TestCollectText(t *testing.T) {
	got := collectText(figmaTree())
	want := []string{"支払一覧", "合計"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("collectText = %v, want %v", got, want)
	}
}

func TestCollectVariants(t *testing.T) {
	got := collectVariants(figmaTree())
	want := []string{"Default", "Empty"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("collectVariants = %v, want %v", got, want)
	}
}

func TestCollectVariants_NestedComponentSet(t *testing.T) {
	tree := map[string]any{"type": "COMPONENT_SET", "children": []any{
		map[string]any{"type": "COMPONENT", "name": "A", "children": []any{
			map[string]any{"type": "COMPONENT_SET", "children": []any{
				map[string]any{"type": "COMPONENT", "name": "B"},
			}},
		}},
	}}
	got := collectVariants(tree)
	want := []string{"A", "B"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("入れ子の COMPONENT_SET も拾う: %v, want %v", got, want)
	}
}
