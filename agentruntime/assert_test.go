/*
Copyright 2026 The K8sAgentMesh Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package agentruntime

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// assert / require are minimal stdlib shims covering exactly the testify
// assertions the upstreamed codex parse tests use. The mesh repo keeps
// stdlib-only tests; these files compile only under `go test`, so the
// shims add no production dependency. assert.* records failures and
// continues; require.* aborts the test (testify semantics).
//nolint:unused // testify-compat shim: completeness beats minimalism

type assertShim struct{}

//nolint:unused // testify-compat shim
type requireShim struct{}

var assert assertShim
var require requireShim

func msgOf(msg []any) string {
	if len(msg) == 0 {
		return ""
	}
	if s, ok := msg[0].(string); ok {
		return fmt.Sprintf(" — "+s, msg[1:]...)
	}
	return fmt.Sprintf(" — %v", msg[0])
}

func eq(t *testing.T, want, got any, fatal bool, msg []any) {
	t.Helper()
	if !reflect.DeepEqual(want, got) {
		m := "Equal:\n want: %#v\n got:  %#v" + msgOf(msg)
		if fatal {
			t.Fatalf("require.%s", m)
		}
		t.Errorf("assert.%s", m)
	}
}

func (assertShim) Equal(t *testing.T, want, got any, msg ...any)  { eq(t, want, got, false, msg) }
func (requireShim) Equal(t *testing.T, want, got any, msg ...any) { eq(t, want, got, true, msg) }

func (assertShim) Empty(t *testing.T, x any, msg ...any)  { empty(t, x, false, msg) }
func (requireShim) Empty(t *testing.T, x any, msg ...any) { empty(t, x, true, msg) }

func empty(t *testing.T, x any, fatal bool, msg []any) {
	t.Helper()
	emptyVal := false
	switch v := x.(type) {
	case string:
		emptyVal = v == ""
	case nil:
		emptyVal = true
	default:
		rv := reflect.ValueOf(x)
		switch rv.Kind() {
		case reflect.Slice, reflect.Map, reflect.Array, reflect.Chan:
			emptyVal = rv.Len() == 0
		default:
			emptyVal = reflect.DeepEqual(x, reflect.Zero(rv.Type()).Interface())
		}
	}
	if !emptyVal {
		m := fmt.Sprintf("Empty: %#v", x) + msgOf(msg)
		if fatal {
			t.Fatalf("require.%s", m)
		}
		t.Errorf("assert.%s", m)
	}
}

func contains(t *testing.T, s, substr string, fatal bool, msg []any) {
	t.Helper()
	if !strings.Contains(s, substr) {
		m := fmt.Sprintf("Contains: %q lacks %q", s, substr) + msgOf(msg)
		if fatal {
			t.Fatalf("require.%s", m)
		}
		t.Errorf("assert.%s", m)
	}
}

func (assertShim) Contains(t *testing.T, s, substr string, msg ...any) {
	contains(t, s, substr, false, msg)
}
func (requireShim) Contains(t *testing.T, s, substr string, msg ...any) {
	contains(t, s, substr, true, msg)
}

func boolCheck(t *testing.T, ok, want bool, name string, fatal bool, msg []any) {
	t.Helper()
	if ok != want {
		m := fmt.Sprintf("%s: got %v", name, ok) + msgOf(msg)
		if fatal {
			t.Fatalf("require.%s", m)
		}
		t.Errorf("assert.%s", m)
	}
}

func (assertShim) True(t *testing.T, ok bool, msg ...any) { boolCheck(t, ok, true, "True", false, msg) }
func (assertShim) False(t *testing.T, ok bool, msg ...any) {
	boolCheck(t, ok, false, "False", false, msg)
}
func (requireShim) True(t *testing.T, ok bool, msg ...any) { boolCheck(t, ok, true, "True", true, msg) }
func (requireShim) False(t *testing.T, ok bool, msg ...any) {
	boolCheck(t, ok, false, "False", true, msg)
}

func noError(t *testing.T, err error, fatal bool, msg []any) {
	t.Helper()
	if err != nil {
		m := "NoError: " + err.Error() + msgOf(msg)
		if fatal {
			t.Fatalf("require.%s", m)
		}
		t.Errorf("assert.%s", m)
	}
}

func (assertShim) NoError(t *testing.T, err error, msg ...any)  { noError(t, err, false, msg) }
func (requireShim) NoError(t *testing.T, err error, msg ...any) { noError(t, err, true, msg) }

func jsonEq(t *testing.T, want, got string, fatal bool) {
	t.Helper()
	var w, g any
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("JSONEq: bad want JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(got), &g); err != nil {
		t.Fatalf("JSONEq: bad got JSON: %v", err)
	}
	if !reflect.DeepEqual(w, g) {
		m := "JSONEq:\n want: " + want + "\n got:  " + got
		if fatal {
			t.Fatalf("require.%s", m)
		}
		t.Errorf("assert.%s", m)
	}
}

func (assertShim) JSONEq(t *testing.T, want, got string, _ ...any)  { jsonEq(t, want, got, false) }
func (requireShim) JSONEq(t *testing.T, want, got string, _ ...any) { jsonEq(t, want, got, true) }

func isNilVal(x any) bool {
	if x == nil {
		return true
	}
	rv := reflect.ValueOf(x)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func, reflect.Interface:
		return rv.IsNil()
	}
	return false
}

func notNil(t *testing.T, x any, msg []any) {
	t.Helper()
	if isNilVal(x) {
		t.Fatalf("require.NotNil: value is nil%s", msgOf(msg))
	}
}

func (requireShim) NotNil(t *testing.T, x any, msg ...any) { notNil(t, x, msg) }
func (assertShim) NotNil(t *testing.T, x any, msg ...any) {
	t.Helper()
	if isNilVal(x) {
		t.Errorf("assert.NotNil: value is nil%s", msgOf(msg))
	}
}

func lenOf(t *testing.T, x any) int {
	t.Helper()
	switch v := x.(type) {
	case string:
		return len(v)
	default:
		rv := reflect.ValueOf(x)
		switch rv.Kind() {
		case reflect.Slice, reflect.Map, reflect.Array, reflect.Chan:
			return rv.Len()
		}
	}
	t.Fatalf("require.Len: unsupported type %T", x)
	return -1
}

func (requireShim) Len(t *testing.T, x any, n int, msg ...any) {
	t.Helper()
	if got := lenOf(t, x); got != n {
		t.Fatalf("require.Len: got %d, want %d%s", got, n, msgOf(msg))
	}
}

func (assertShim) Len(t *testing.T, x any, n int, msg ...any) {
	t.Helper()
	if got := lenOf(t, x); got != n {
		t.Errorf("assert.Len: got %d, want %d%s", got, n, msgOf(msg))
	}
}
