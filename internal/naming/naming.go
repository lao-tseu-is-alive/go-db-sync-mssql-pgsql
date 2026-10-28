// Package naming reproduces the legacy table-name transformation.
package naming

import (
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var pythonLower = cases.Lower(language.Und)

var acronymReplacements = []struct {
	old string
	new string
}{
	{"AGF", "Agf"}, {"EGID", "Egid"}, {"ARCH", "Arch"}, {"ASST", "Asst"},
	{"CID", "Cid"}, {"CIL", "Cil"}, {"ESTRID", "Estrid"}, {"ES", "Es"},
	{"GC", "Gc"}, {"GEO", "Geo"}, {"CCJP", "CcJp"}, {"CH", "Ch"},
	{"CN", "Cn"}, {"RCB", "Rcb"}, {"DMZ", "Dmz"}, {"ISO", "Iso"},
	{"NT", "Nt"}, {"OPC", "Opc"}, {"POLC", "Polc"}, {"QSE", "Qse"},
	{"RM", "Rm"}, {"SAP", "Sap"}, {"SCC", "Scc"}, {"SPD", "Spd"},
	{"URB", "Urb"},
}

// StandardTableName applies the legacy ordered acronym substitutions.
func StandardTableName(name string) string {
	for _, replacement := range acronymReplacements {
		name = strings.ReplaceAll(name, replacement.old, replacement.new)
	}
	return name
}

// TableName applies the exact legacy table-name transformation.
func TableName(name string) string {
	name = StandardTableName(name)
	var result strings.Builder
	for index := 0; index < len(name); {
		if !isASCIICapital(name[index]) {
			result.WriteByte(name[index])
			index++
			continue
		}

		end := index + 1
		for end < len(name) && isASCIICapital(name[end]) {
			end++
		}
		if index == 0 {
			result.WriteByte(name[index])
			index++
		}
		if index < end {
			result.WriteByte('_')
			result.WriteString(name[index:end])
		}
		index = end
	}
	return pythonLower.String(result.String())
}

func isASCIICapital(value byte) bool {
	return value >= 'A' && value <= 'Z'
}
