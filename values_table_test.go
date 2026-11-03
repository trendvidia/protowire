// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package protowire_test

import (
	"bytes"
	"encoding/hex"
	"flag"
	"fmt"
	"math"
	"math/big"
	"os"
	"strings"
	"testing"

	"github.com/trendvidia/protowire-go/encoding/pb"
)

// The struct-binding table (#509): values a port's struct API (Go
// encoding/pb structs, Java's Pb, and so on) writes, with the bytes
// protowire-go writes for each. A port's golden test holds a twin of every
// struct below and compares its bytes with testdata/values/values.tsv. No
// gate could drive a struct API across ports, so this is the shared half.
//
// go test -run TestValuesTable -update rewrites the table from protowire-go.
var updateValues = flag.Bool("update", false, "rewrite testdata/values/values.tsv")

type vInts struct {
	A  int64            `protowire:"1"`
	B  int64            `protowire:"2"`
	C  int32            `protowire:"3"`
	M  map[string]int64 `protowire:"4"`
	MI map[string]int32 `protowire:"5"`
}
type vAge struct {
	Age int32 `protowire:"1"`
}
type vMinLong struct {
	V int64 `protowire:"1"`
}
type vZigzag struct {
	D int64 `protowire:"1,zigzag"`
	E int32 `protowire:"2,zigzag"`
}
type vPacked struct {
	Xs []int32   `protowire:"1"`
	Ys []int64   `protowire:"2"`
	Bs []bool    `protowire:"3"`
	Ds []float64 `protowire:"4"`
	Fs []float32 `protowire:"5"`
}
type vZeroMapEntry struct {
	M map[string]string `protowire:"1"`
	Z map[string]int32  `protowire:"2"`
	K map[int32]string  `protowire:"3"`
}
type vDecimal struct {
	D *big.Rat `protowire:"1"`
}
type vDecimals struct {
	Ds []*big.Rat `protowire:"1"`
}
type vLeaf struct {
	Name string `protowire:"1"`
	N    int32  `protowire:"2"`
}
type vNested struct {
	S  string   `protowire:"1"`
	Ls []vLeaf  `protowire:"2"`
	Ss []string `protowire:"3"`
}
type vZeros struct {
	I  int32   `protowire:"1"`
	S  string  `protowire:"2"`
	B  bool    `protowire:"3"`
	D  float64 `protowire:"4"`
	Bs []byte  `protowire:"5"`
	U  uint64  `protowire:"6"`
}
type vBigNums struct {
	I *big.Int   `protowire:"1"`
	D *big.Rat   `protowire:"2"`
	F *big.Float `protowire:"3"`
}
type vBigInts struct {
	Is []*big.Int `protowire:"1"`
}
type vBigFloatMap struct {
	M map[string]*big.Float `protowire:"1"`
}
type vBigFloat struct {
	F *big.Float `protowire:"1"`
}

// valueCase is one row: a name (the struct's twin in each port), the value
// in words a port can rebuild it from, and the value itself.
type valueCase struct {
	name, value string
	v           any
}

func valueCases() []valueCase {
	return []valueCase{
		{"Ints", "Ints{a: -1, b: -1, c: -1, m: {k: -1}, mi: {k: -1}}", &vInts{A: -1, B: -1, C: -1, M: map[string]int64{"k": -1}, MI: map[string]int32{"k": -1}}},
		{"Age", "Age{age: 30}", &vAge{Age: 30}},
		{"MinLong", "MinLong{v: -2^63}", &vMinLong{V: math.MinInt64}},
		{"Zigzag", "Zigzag{d: -1, e: -2}, both zigzag", &vZigzag{D: -1, E: -2}},
		{"Packed", "Packed{xs: [1, 0, -1], ys: [300], bs: [true, false], ds: [1.5], fs: [-2.5]}", &vPacked{Xs: []int32{1, 0, -1}, Ys: []int64{300}, Bs: []bool{true, false}, Ds: []float64{1.5}, Fs: []float32{-2.5}}},
		{"ZeroMapEntry", `ZeroMapEntry{m: {"": ""}, z: {zero: 0}, k: {0: "v"}}`, &vZeroMapEntry{M: map[string]string{"": ""}, Z: map[string]int32{"zero": 0}, K: map[int32]string{0: "v"}}},
		{"Decimal", "Decimal{d: 3.1415}", &vDecimal{D: big.NewRat(31415, 10000)}},
		// A language value is written reduced (draft -01 § PXF Annotations,
		// #508): 1.00 and 1 are the same bytes.
		{"DecimalOne", "Decimal{d: 1.00}", &vDecimal{D: big.NewRat(1, 1)}},
		{"DecimalFiveE2", "Decimal{d: 5E+2}", &vDecimal{D: big.NewRat(500, 1)}},
		{"DecimalFifteen", "Decimal{d: 15.00}", &vDecimal{D: big.NewRat(15, 1)}},
		{"DecimalZero", "Decimal{d: 0.00}", &vDecimal{D: new(big.Rat)}},
		{"DecimalNegative", "Decimal{d: -1.50}", &vDecimal{D: big.NewRat(-3, 2)}},
		{"Decimals", "Decimals{ds: [0.00, 1.00, 5E+2]}", &vDecimals{Ds: []*big.Rat{new(big.Rat), big.NewRat(1, 1), big.NewRat(500, 1)}}},
		{"Nested", `Nested{s: "x", ls: [Leaf{}, Leaf{name: "l", n: -3}], ss: ["", "s"]}`, &vNested{S: "x", Ls: []vLeaf{{}, {Name: "l", N: -3}}, Ss: []string{"", "s"}}},
		// Zero scalars are omitted; a zero element of a list is not
		// (protowire-java#156).
		{"Zeros", `Zeros{i: 0, s: "", b: false, d: 0, bs: empty, u: 0}`, &vZeros{}},
		// Null and zero big numbers (protowire-java#159). protowire-go
		// writes neither: a zero big number is a zero value, omitted like a
		// zero scalar. A zero element of a list is written, as an empty
		// message.
		{"BigNumsNull", "BigNums{i: null, d: null, f: null}", &vBigNums{}},
		{"BigNumsZero", "BigNums{i: 0, d: 0, f: 0 at prec 0}", &vBigNums{I: new(big.Int), D: new(big.Rat), F: new(big.Float)}},
		{"BigNumsNegative", "BigNums{i: -12345678901234567890, d: -0.5, f: -1.5 at prec 53}", &vBigNums{I: mustBigInt("-12345678901234567890"), D: big.NewRat(-1, 2), F: big.NewFloat(-1.5)}},
		{"BigIntsZeroElement", "BigInts{is: [0, 1]}", &vBigInts{Is: []*big.Int{new(big.Int), big.NewInt(1)}}},
		// A null element or map value is the empty message, so a list keeps
		// its length and a map entry keeps its value field (#78).
		{"BigIntsNullElement", "BigInts{is: [1, null]}", &vBigInts{Is: []*big.Int{big.NewInt(1), nil}}},
		{"DecimalsNullElement", "Decimals{ds: [1, null]}", &vDecimals{Ds: []*big.Rat{big.NewRat(1, 1), nil}}},
		{"BigFloatMapNullValue", "BigFloatMap{m: {k: null}}", &vBigFloatMap{M: map[string]*big.Float{"k": nil}}},
		// The mantissa is written widened to prec bits, whatever form the
		// value was built from.
		{"BigFloatPrec64", "BigFloat{f: 1.5 at prec 64}", &vBigFloat{F: new(big.Float).SetPrec(64).SetFloat64(1.5)}},
	}
}

func mustBigInt(s string) *big.Int {
	i, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic(s)
	}
	return i
}

func renderValues(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	b.WriteString("# The struct-binding table (protowire#509): <case>\\t<hex>\\t<value>.\n")
	b.WriteString("# <hex> is what protowire-go's encoding/pb writes for <value>; an empty\n")
	b.WriteString("# column is an empty message. Generated by values_table_test.go -update.\n")
	for _, c := range valueCases() {
		if strings.ContainsAny(c.name+c.value, "\t\n") {
			t.Fatalf("%s: a tab or newline in the row", c.name)
		}
		out, err := pb.Marshal(c.v)
		if err != nil {
			t.Fatalf("%s: protowire-go refused the value: %v", c.name, err)
		}
		fmt.Fprintf(&b, "%s\t%s\t%s\n", c.name, hex.EncodeToString(out), c.value)
	}
	return b.Bytes()
}

func TestValuesTable(t *testing.T) {
	const path = "testdata/values/values.tsv"
	got := renderValues(t)
	if *updateValues {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test -run TestValuesTable -update)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s is not what the pinned protowire-go writes; run go test -run TestValuesTable -update and read the diff", path)
	}
}
