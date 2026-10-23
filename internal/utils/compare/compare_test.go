/*
Copyright 2026 The Crossplane Authors.

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

package compare

import (
	"fmt"
	"math"
	"math/rand"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

type scaling struct {
	Max *float64
	Min *float64
}

type tag struct {
	Key   *string
	Value *string
}

func TestIsSubset(t *testing.T) {
	now := time.Now()
	cases := map[string]struct {
		desired  any
		observed any
		want     bool
	}{
		"NilPointerMatchesAnything":  {desired: (*string)(nil), observed: ptr.To("a"), want: true},
		"EqualPointers":              {desired: ptr.To("a"), observed: ptr.To("a"), want: true},
		"DifferentPointers":          {desired: ptr.To(int64(1)), observed: ptr.To(int64(2)), want: false},
		"SetVsNil":                   {desired: ptr.To(false), observed: (*bool)(nil), want: false},
		"FloatEqual":                 {desired: ptr.To(0.5), observed: ptr.To(0.5), want: true},
		"NilSliceMatchesAnything":    {desired: []*string(nil), observed: []*string{ptr.To("a")}, want: true},
		"EmptySliceMustBeEmpty":      {desired: []*string{}, observed: []*string{ptr.To("a")}, want: false},
		"EmptySliceVsNil":            {desired: []*string{}, observed: []*string(nil), want: true},
		"SliceOrderDoesNotMatter":    {desired: []*string{ptr.To("a"), ptr.To("b")}, observed: []*string{ptr.To("b"), ptr.To("a")}, want: true},
		"SliceDuplicates":            {desired: []*string{ptr.To("a"), ptr.To("a")}, observed: []*string{ptr.To("a"), ptr.To("b")}, want: false},
		"SliceLength":                {desired: []*string{ptr.To("a")}, observed: []*string{ptr.To("a"), ptr.To("b")}, want: false},
		"StructPartial":              {desired: &scaling{Max: ptr.To(4.0)}, observed: &scaling{Max: ptr.To(4.0), Min: ptr.To(0.5)}, want: true},
		"StructDiffers":              {desired: &scaling{Min: ptr.To(1.0)}, observed: &scaling{Max: ptr.To(4.0), Min: ptr.To(0.5)}, want: false},
		"StructVsNil":                {desired: &scaling{Min: ptr.To(1.0)}, observed: (*scaling)(nil), want: false},
		"StructSlice":                {desired: []*tag{{Key: ptr.To("k"), Value: ptr.To("v")}}, observed: []*tag{{Key: ptr.To("k"), Value: ptr.To("v")}}, want: true},
		"StructSliceDiffers":         {desired: []*tag{{Key: ptr.To("k"), Value: ptr.To("v")}}, observed: []*tag{{Key: ptr.To("k"), Value: ptr.To("w")}}, want: false},
		"Map":                        {desired: map[string]*string{"a": ptr.To("1")}, observed: map[string]*string{"a": ptr.To("1")}, want: true},
		"MapMissingKey":              {desired: map[string]*string{"a": ptr.To("1")}, observed: map[string]*string{"b": ptr.To("1")}, want: false},
		"MapExtraKey":                {desired: map[string]*string{"a": ptr.To("1")}, observed: map[string]*string{"a": ptr.To("1"), "b": ptr.To("2")}, want: false},
		"TimeEqual":                  {desired: &metav1.Time{Time: now}, observed: &metav1.Time{Time: now.UTC()}, want: true},
		"TimeDiffers":                {desired: &metav1.Time{Time: now}, observed: &metav1.Time{Time: now.Add(time.Second)}, want: false},
		"UntypedNilObserved":         {desired: ptr.To("a"), observed: nil, want: false},
		"UntypedNilObservedEmpty":    {desired: &scaling{}, observed: nil, want: true},
		"UntypedNilDesired":          {desired: nil, observed: ptr.To("a"), want: true},
		"ScalarValues":               {desired: "a", observed: "a", want: true},
		"ScalarValuesDiffer":         {desired: "a", observed: "b", want: false},
		"NilObservedWithEmptyStruct": {desired: &scaling{}, observed: (*scaling)(nil), want: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := IsSubset(tc.desired, tc.observed); got != tc.want {
				t.Errorf("IsSubset(%v, %v): want %v, got %v", tc.desired, tc.observed, tc.want, got)
			}
		})
	}
}

func TestWithout(t *testing.T) {
	got := Without([]string{"a", "b", "c"}, "b", "x")
	if diff := cmp.Diff([]string{"a", "c"}, got); diff != "" {
		t.Errorf("Without(...): -want, +got:\n%s", diff)
	}
}

func TestKMSKeyIDEqual(t *testing.T) {
	arn := "arn:aws:kms:eu-central-1:123456789012:key/1234abcd"
	cases := map[string]struct {
		a, b string
		want bool
	}{
		"Same":         {a: "1234abcd", b: "1234abcd", want: true},
		"IDAndARN":     {a: "1234abcd", b: arn, want: true},
		"ARNAndID":     {a: arn, b: "1234abcd", want: true},
		"OtherKey":     {a: "5678", b: arn, want: false},
		"PartialMatch": {a: "abcd", b: arn, want: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := KMSKeyIDEqual(tc.a, tc.b); got != tc.want {
				t.Errorf("KMSKeyIDEqual(%q, %q): want %v, got %v", tc.a, tc.b, tc.want, got)
			}
		})
	}
}

func TestMessage(t *testing.T) {
	if got := Message(nil); got != "" {
		t.Errorf("Message(nil): want empty, got %q", got)
	}
	want := "spec.forProvider fields differ from the observed state: a, b"
	if got := Message([]string{"a", "b"}); got != want {
		t.Errorf("Message(...): want %q, got %q", want, got)
	}
}

type item struct {
	A *int64
	B *string
}

func TestIsSubsetMatching(t *testing.T) {
	a1 := &item{A: ptr.To(int64(1)), B: ptr.To("a")}
	a2 := &item{A: ptr.To(int64(2)), B: ptr.To("a")}
	onlyB := &item{B: ptr.To("a")}
	i0, i1 := &item{A: ptr.To(int64(0))}, &item{A: ptr.To(int64(1))}
	only1 := &item{A: ptr.To(int64(1))}
	cases := map[string]struct {
		desired  any
		observed any
		want     bool
	}{
		"GreedyCounterExample": {desired: []*item{onlyB, a1}, observed: []*item{a1, a2}, want: true},
		"GreedyReversed":       {desired: []*item{a1, onlyB}, observed: []*item{a1, a2}, want: true},
		"NoPerfectMatching":    {desired: []*item{only1, only1}, observed: []*item{{A: ptr.To(int64(1))}, {A: ptr.To(int64(2))}}, want: false},
		// Two leftovers after first fit: the second search must start fresh.
		"TwoLeftoversAfterFirstFit": {
			desired:  []*item{{}, {}, i0, i1},
			observed: []*item{i0, i1, {A: ptr.To(int64(8))}, {A: ptr.To(int64(9))}},
			want:     true,
		},
		// Found by the fuzzer: a search must not inherit the previous one's marks.
		"SearchesAreIndependent": {
			desired: []*item{
				{A: ptr.To(int64(0))}, {A: ptr.To(int64(0)), B: ptr.To("a")}, {}, {A: ptr.To(int64(1))},
				{B: ptr.To("a")}, {A: ptr.To(int64(2))},
			},
			observed: []*item{
				{A: ptr.To(int64(0))}, {A: ptr.To(int64(0)), B: ptr.To("a")}, {A: ptr.To(int64(2)), B: ptr.To("a")},
				{A: ptr.To(int64(0))}, {A: ptr.To(int64(1)), B: ptr.To("a")}, {A: ptr.To(int64(1))},
			},
			want: true,
		},
		"NestedPermuted": {
			desired: []*struct{ L []*string }{
				{L: []*string{ptr.To("x"), ptr.To("y")}}, {L: []*string{ptr.To("z")}},
			},
			observed: []*struct{ L []*string }{
				{L: []*string{ptr.To("z")}}, {L: []*string{ptr.To("y"), ptr.To("x")}},
			},
			want: true,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := IsSubset(tc.desired, tc.observed); got != tc.want {
				t.Errorf("IsSubset(...): want %v, got %v", tc.want, got)
			}
		})
	}
}

func TestIsSubsetPlainValueLists(t *testing.T) {
	cases := map[string]struct {
		desired  []*string
		observed []*string
		want     bool
	}{
		"NilDesiredElementIsWildcard": {desired: []*string{nil, ptr.To("a")}, observed: []*string{ptr.To("b"), ptr.To("a")}, want: true},
		"WildcardFirstOrLast":         {desired: []*string{ptr.To("a"), nil}, observed: []*string{ptr.To("b"), ptr.To("a")}, want: true},
		"DuplicatesNeedDistinctOnes":  {desired: []*string{ptr.To("a"), ptr.To("a")}, observed: []*string{ptr.To("a"), ptr.To("b")}, want: false},
		"ObservedNilNeedsWildcard":    {desired: []*string{ptr.To("a")}, observed: []*string{nil}, want: false},
		"NilMatchesNil":               {desired: []*string{nil}, observed: []*string{nil}, want: true},
		"WildcardTakesObservedNil":    {desired: []*string{nil, ptr.To("a")}, observed: []*string{ptr.To("a"), nil}, want: true},
		"SameValuesDifferentCounts":   {desired: []*string{ptr.To("a"), ptr.To("a"), ptr.To("b")}, observed: []*string{ptr.To("a"), ptr.To("b"), ptr.To("b")}, want: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := IsSubset(tc.desired, tc.observed); got != tc.want {
				t.Errorf("IsSubset(...): want %v, got %v", tc.want, got)
			}
		})
	}
}

func permutedStrings(n int) (desired, observed []*string) {
	for i := range n {
		observed = append(observed, ptr.To(fmt.Sprintf("value-%d", i)))
	}
	desired = append(desired, observed...)
	rand.New(rand.NewSource(1)).Shuffle(n, func(i, j int) { desired[i], desired[j] = desired[j], desired[i] })
	return desired, observed
}

func TestIsSubsetLargePermutedList(t *testing.T) {
	d, o := permutedStrings(10000)
	if !IsSubset(d, o) {
		t.Error("a permuted list of 10000 strings must be a subset")
	}
}

func TestIsSubsetLongListWithWildcard(t *testing.T) {
	// Longer than the exact struct matching goes, so only the plain-value
	// path can see that the wildcard must take the one leftover value.
	var d, o []*string
	d = append(d, nil)
	for i := 1; i <= 2000; i++ {
		v := ptr.To(fmt.Sprintf("value-%d", i%2000))
		o = append(o, v)
		if i < 2000 {
			d = append(d, v)
		}
	}
	if !IsSubset(d, o) {
		t.Error("the wildcard must take the leftover value")
	}
}

func BenchmarkIsSubsetLargeSlices(b *testing.B) {
	d, o := permutedStrings(10000)
	b.Run("Strings10000", func(b *testing.B) {
		for range b.N {
			IsSubset(d, o)
		}
	})
	os := make([]*item, 200)
	for i := range os {
		os[i] = &item{A: ptr.To(int64(i)), B: ptr.To("a")}
	}
	ds := append([]*item(nil), os...)
	rand.New(rand.NewSource(2)).Shuffle(len(ds), func(i, j int) { ds[i], ds[j] = ds[j], ds[i] })
	md, mo := nothingFits(5000)
	b.Run("StructsMismatch5000", func(b *testing.B) {
		for range b.N {
			IsSubset(md, mo)
		}
	})
	b.Run("Structs200", func(b *testing.B) {
		for range b.N {
			IsSubset(ds, os)
		}
	})
}

func TestIsSubsetTypeMismatch(t *testing.T) {
	type withAny struct{ V any }
	type withFunc struct{ F func() }
	fn := func() {}
	cases := map[string]struct {
		desired  any
		observed any
		want     bool
	}{
		"TopLevelMismatch":          {desired: &item{A: ptr.To(int64(1))}, observed: "x", want: false},
		"InterfaceDynamicMismatch":  {desired: withAny{V: item{A: ptr.To(int64(1))}}, observed: withAny{V: "s"}, want: false},
		"InterfaceSameDynamicType":  {desired: withAny{V: "s"}, observed: withAny{V: "s"}, want: true},
		"NilDesiredIgnoresMismatch": {desired: (*item)(nil), observed: "x", want: true},
		"NilSliceIgnoresMismatch":   {desired: []string(nil), observed: 5, want: true},
		"SliceVsScalar":             {desired: []string{"a"}, observed: "a", want: false},
		"FuncIsNeverEqual":          {desired: withFunc{F: fn}, observed: withFunc{F: fn}, want: false},
		"NilFuncIsUnset":            {desired: withFunc{}, observed: withFunc{F: fn}, want: true},
		"ArrayByPosition":           {desired: [2]int{1, 2}, observed: [2]int{1, 2}, want: true},
		"ArrayOrderMatters":         {desired: [2]int{1, 2}, observed: [2]int{2, 1}, want: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("IsSubset panicked: %v", r)
				}
			}()
			if got := IsSubset(tc.desired, tc.observed); got != tc.want {
				t.Errorf("IsSubset(...): want %v, got %v", tc.want, got)
			}
		})
	}
}

type outer struct{ In *item }

type withLists struct {
	L []*string
	M map[string]*string
}

func TestIsSubsetNilAndEmpty(t *testing.T) {
	cases := map[string]struct {
		desired  any
		observed any
		want     bool
	}{
		"EmptyStructVsNil":         {desired: &item{}, observed: (*item)(nil), want: true},
		"SetFieldVsNil":            {desired: &item{A: ptr.To(int64(1))}, observed: (*item)(nil), want: false},
		"NestedEmptyStructVsNil":   {desired: &outer{In: &item{}}, observed: (*outer)(nil), want: true},
		"NestedEmptyVsNilInner":    {desired: &outer{In: &item{}}, observed: &outer{}, want: true},
		"NestedSetFieldVsNilInner": {desired: &outer{In: &item{B: ptr.To("x")}}, observed: &outer{}, want: false},
		"EmptySliceFieldVsNil":     {desired: &withLists{L: []*string{}}, observed: (*withLists)(nil), want: true},
		"EmptyStringVsNil":         {desired: ptr.To(""), observed: (*string)(nil), want: false},
		"FalseVsNil":               {desired: ptr.To(false), observed: (*bool)(nil), want: false},
		"ZeroIntVsNil":             {desired: ptr.To(int64(0)), observed: (*int64)(nil), want: false},
		"EmptySliceVsNil":          {desired: []*string{}, observed: []*string(nil), want: true},
		"EmptySliceVsEmpty":        {desired: []*string{}, observed: []*string{}, want: true},
		"EmptySliceVsOne":          {desired: []*string{}, observed: []*string{ptr.To("x")}, want: false},
		"NilSliceVsOne":            {desired: []*string(nil), observed: []*string{ptr.To("x")}, want: true},
		"EmptyMapVsNil":            {desired: map[string]*string{}, observed: map[string]*string(nil), want: true},
		"MapNilValueKeyAbsent":     {desired: map[string]*string{"k": nil}, observed: map[string]*string{"j": ptr.To("v")}, want: false},
		"MapNilValueKeyPresent":    {desired: map[string]*string{"k": nil}, observed: map[string]*string{"k": ptr.To("v")}, want: true},
		"MapValueDiffers":          {desired: map[string]*string{"k": ptr.To("a")}, observed: map[string]*string{"k": ptr.To("b")}, want: false},
		"TimeAcrossZones":          {desired: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), observed: time.Date(2026, 1, 1, 13, 0, 0, 0, time.FixedZone("x", 3600)), want: true},
		"NaNIsNotReflexive":        {desired: ptr.To(math.NaN()), observed: ptr.To(math.NaN()), want: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := IsSubset(tc.desired, tc.observed); got != tc.want {
				t.Errorf("IsSubset(...): want %v, got %v", tc.want, got)
			}
		})
	}
}

func TestIsSubsetIgnoresUnexportedFields(t *testing.T) {
	type hidden struct {
		Shown  int
		hidden int
	}
	if !IsSubset(hidden{Shown: 1, hidden: 1}, hidden{Shown: 1, hidden: 2}) {
		t.Error("unexported fields must not count")
	}
}

// wildcardLate returns lists that only match when the empty element, which
// first fit pairs too early, is moved to the one observed item left over at
// the end. Matching them needs exactly one re-routing.
func wildcardLate(n int) (desired, observed []*item) {
	for i := 1; i < n; i++ {
		observed = append(observed, &item{A: ptr.To(int64(i))})
	}
	observed = append(observed, &item{A: ptr.To(int64(0))})
	for i := 1; i < n-1; i++ {
		desired = append(desired, observed[i-1])
	}
	desired = append(desired, &item{}, observed[n-2])
	return desired, observed
}

type heavyItem struct {
	L []*string
	A *int64
}

// heavyChain returns lists where one empty element, placed first, has to be
// moved once for every element after it, and every comparison of two elements
// has to walk a list of m strings.
func heavyChain(n, m int) (desired, observed []*heavyItem) {
	list := make([]*string, m)
	for i := range list {
		list[i] = ptr.To(fmt.Sprintf("string-%d", i))
	}
	desired = append(desired, &heavyItem{})
	for i := 1; i < n; i++ {
		desired = append(desired, &heavyItem{L: list, A: ptr.To(int64(i))})
		observed = append(observed, &heavyItem{L: list, A: ptr.To(int64(i))})
	}
	observed = append(observed, &heavyItem{L: list, A: ptr.To(int64(0))})
	return desired, observed
}

// used reports how much of the work budget IsSubset(d, o) spends.
func used(d, o any) int {
	w := &walker{left: workBudget}
	w.subset(reflect.ValueOf(d), reflect.ValueOf(o))
	return workBudget - w.left
}

func TestIsSubsetWorkIsBounded(t *testing.T) {
	// A match that needs about a million re-routings is given up on: the
	// lists are reported as different, and quickly. Without the budget this
	// takes over a minute.
	d, o := heavyChain(1024, 300)
	start := time.Now()
	if IsSubset(d, o) {
		t.Error("the work budget should have run out and reported a difference")
	}
	if took := time.Since(start); took > 10*time.Second {
		t.Errorf("a heavy list took %v", took)
	}
}

type guardSpec struct {
	A []*string
	B []*item
}

// A spent budget must never end in "up to date", even when the field that
// runs it dry is followed by one whose comparison costs nothing.
func TestIsSubsetNeverMatchesAfterBudgetRunsOut(t *testing.T) {
	// A counted list costs 2 units per element: this leaves about 100 000.
	strs := make([]*string, (workBudget-100000)/2)
	for i := range strs {
		strs[i] = ptr.To(strconv.Itoa(i))
	}
	// The second field needs a table of 1024*1024/8 units; nil desired
	// elements then match for free, so only the budget check can fail it.
	nils := make([]*item, 1024)
	items := make([]*item, 1024)
	for i := range items {
		items[i] = &item{}
	}
	if IsSubset(guardSpec{A: strs, B: nils}, guardSpec{A: strs, B: items}) {
		t.Error("a call that ran out of budget must report a difference")
	}
}

// The budget is the only bound on the work, so its charges are pinned with
// exact numbers instead of timings.
func TestIsSubsetWorkCharges(t *testing.T) {
	strs := func(vs ...string) []*string {
		out := make([]*string, len(vs))
		for i, v := range vs {
			out[i] = ptr.To(v)
		}
		return out
	}
	inOrder := make([]*item, 16)
	for i := range inOrder {
		inOrder[i] = &item{A: ptr.To(int64(i))}
	}
	late4d, late4o := wildcardLate(4)
	cases := map[string]struct {
		desired, observed any
		want              int
	}{
		// 1 for the list, 1 per element of each list for counting.
		"CountedList": {desired: strs("a", "b", "c"), observed: strs("c", "b", "a"), want: 1 + 3 + 3},
		// 1 for the list, 16*16/8 for the table, 4 per element compared
		// once (pointer, struct, pointer, int; the nil field is free).
		"TableAndFirstFit": {desired: inOrder, observed: inOrder, want: 1 + 32 + 16*4},
		// 1 list, 2 table, 14 first fit (4+4+2+4), then one re-routing:
		// 4 scanned for a free partner, 3 scanned moving others and 3
		// compares of 4, then the moved element scans 4 and compares for 2.
		"Rerouting": {desired: late4d, observed: late4o, want: 1 + 2 + 14 + 4 + 3 + 12 + 4 + 2},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := used(tc.desired, tc.observed); got != tc.want {
				t.Errorf("units spent: want %d, got %d", tc.want, got)
			}
		})
	}
}

func TestIsSubsetBudgetMeansDiffers(t *testing.T) {
	d, o := heavyChain(1024, 0)
	if IsSubset(d, o) {
		t.Error("a match that needs more work than the budget must report a difference")
	}
}

func TestIsSubsetRealisticListsStayInBudget(t *testing.T) {
	observed := make([]*tag, 50)
	for i := range observed {
		observed[i] = &tag{Key: ptr.To(fmt.Sprintf("key-%d", i)), Value: ptr.To("v")}
	}
	reversed := slices.Clone(observed)
	slices.Reverse(reversed)
	// A wildcard tag in front is the worst case for first fit.
	wild := append([]*tag{{}}, observed[1:]...)
	wildObserved := append(slices.Clone(observed[1:]), observed[0])
	cases := map[string]struct{ desired, observed []*tag }{
		"FiftyTagsReversed": {desired: reversed, observed: observed},
		"FiftyTagsWildcard": {desired: wild, observed: wildObserved},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if !IsSubset(tc.desired, tc.observed) {
				t.Error("want a match")
			}
			if got := used(tc.desired, tc.observed); got > workBudget/50 {
				t.Errorf("spent %d of %d units: too close to the budget", got, workBudget)
			}
		})
	}
}

// nothingFits returns n desired items and n observed items that never match.
func nothingFits(n int) (desired, observed []*item) {
	for i := range n {
		desired = append(desired, &item{A: ptr.To(int64(i))})
		observed = append(observed, &item{A: ptr.To(int64(n + i))})
	}
	return desired, observed
}

func TestIsSubsetMismatchIsCheap(t *testing.T) {
	d, o := nothingFits(5000)
	start := time.Now()
	if IsSubset(d, o) {
		t.Fatal("nothing fits, so the lists must differ")
	}
	// Giving up on the first element that fits nothing takes well under a
	// millisecond; trying every pair takes seconds.
	if took := time.Since(start); took > 250*time.Millisecond {
		t.Errorf("a mismatch in a long list took %v", took)
	}
}

func TestIsSubsetMatchingLimit(t *testing.T) {
	d, o := wildcardLate(1024)
	if !IsSubset(d, o) {
		t.Error("a list of 1024 structs must be matched exactly")
	}
	// Longer lists only get the first-fit pairing, which is order dependent.
	d, o = wildcardLate(1025)
	if IsSubset(d, o) {
		t.Error("a list of 1025 structs is expected to hit the documented limit")
	}
}
