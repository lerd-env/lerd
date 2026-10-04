package ui

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// inlineBindings puts a query's bindings in place of its ? placeholders, the
// way the dashboard's Copy SQL does, so a captured query can be run locally.
// A ? inside a quoted string or identifier is left alone, and a ? past the
// last binding stays as it is.
func inlineBindings(sql string, bindings []interface{}) string {
	if len(bindings) == 0 {
		return sql
	}
	var b strings.Builder
	var quote byte
	next := 0
	for i := 0; i < len(sql); i++ {
		ch := sql[i]
		if quote != 0 {
			b.WriteByte(ch)
			switch {
			case ch == '\\' && i+1 < len(sql):
				i++
				b.WriteByte(sql[i])
			case ch == quote && i+1 < len(sql) && sql[i+1] == quote:
				i++
				b.WriteByte(sql[i])
			case ch == quote:
				quote = 0
			}
			continue
		}
		switch {
		case ch == '\'' || ch == '"' || ch == '`':
			quote = ch
			b.WriteByte(ch)
		case ch == '?' && next < len(bindings):
			b.WriteString(sqlLiteral(bindings[next]))
			next++
		default:
			b.WriteByte(ch)
		}
	}
	return b.String()
}

func sqlLiteral(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case bool:
		if x {
			return "1"
		}
		return "0"
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return quoteSQL(strconv.FormatFloat(x, 'g', -1, 64))
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int, int64, json.Number:
		return toString(x)
	case string:
		return quoteSQL(x)
	default:
		b, _ := json.Marshal(x)
		return quoteSQL(string(b))
	}
}

func toString(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func quoteSQL(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
