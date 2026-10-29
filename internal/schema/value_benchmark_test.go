package schema

import "testing"

var benchmarkValue any

func BenchmarkConvertValue(b *testing.B) {
	benchmarks := []struct {
		name   string
		column Column
		value  any
	}{
		{name: "direct-integer", column: Column{Conversion: ConversionDirect}, value: int64(42)},
		{name: "money", column: Column{Conversion: ConversionMoney}, value: []byte("123456789012345.6789")},
		{name: "decimal", column: Column{Conversion: ConversionDecimal}, value: []byte("12345678901234567890.123456789012345678")},
		{name: "uuid", column: Column{Conversion: ConversionUUID}, value: "6f9619ff-8b86-d011-b42d-00c04fc964ff"},
		{name: "rowversion", column: Column{Conversion: ConversionRowVersion}, value: []byte{0, 0, 0, 0, 0, 0, 7, 0xd3}},
	}
	for _, benchmark := range benchmarks {
		benchmark := benchmark
		b.Run(benchmark.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				value, err := ConvertValue(benchmark.column, benchmark.value)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkValue = value
			}
		})
	}
}
