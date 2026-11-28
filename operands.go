package fql

import (
	"strconv"
	"strings"
)

// OperandKind records the lexical form of a condition's right-hand side.
type OperandKind string

const (
	OperandString OperandKind = "string"
	OperandNumber OperandKind = "number"
	OperandBool   OperandKind = "bool"
	OperandNull   OperandKind = "null"
	OperandBare   OperandKind = "bare"
	OperandList   OperandKind = "list"
)

// Operand preserves authored syntax independently of the legacy decoded Value.
// Source is empty only for synthesized unions or caller-created metadata.
type Operand struct {
	Kind   OperandKind `json:"kind"`
	Source string      `json:"source,omitempty"`
	Value  string      `json:"value,omitempty"`
	Items  []Operand   `json:"items,omitempty"`
}

func decimalDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func signedNumber(s string) bool {
	if strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-") {
		s = s[1:]
	}
	index := 0
	for index < len(s) && s[index] >= '0' && s[index] <= '9' {
		index++
	}
	if index == 0 {
		return false
	}
	if index < len(s) && s[index] == '.' {
		index++
		fractionStart := index
		for index < len(s) && s[index] >= '0' && s[index] <= '9' {
			index++
		}
		if index == fractionStart {
			return false
		}
	}
	if index < len(s) && (s[index] == 'e' || s[index] == 'E') {
		index++
		if index < len(s) && (s[index] == '+' || s[index] == '-') {
			index++
		}
		exponentStart := index
		for index < len(s) && s[index] >= '0' && s[index] <= '9' {
			index++
		}
		if index == exponentStart {
			return false
		}
	}
	return index == len(s)
}

// operandFromValue is shared by both extraction views. An incomplete AST value
// has no trustworthy source and therefore no authoritative RHS metadata.
func operandFromValue(value Value) *Operand {
	if value.Source == "" {
		return nil
	}
	operand := &Operand{Source: value.Source}
	if value.IsList() {
		operand.Kind = OperandList
		operand.Items = make([]Operand, 0, len(value.List))
		for _, item := range value.List {
			converted := operandFromValue(item)
			if converted == nil {
				return nil
			}
			operand.Items = append(operand.Items, *converted)
		}
		return operand
	}
	if value.Quoted {
		operand.Kind, operand.Value = OperandString, value.Scalar
		return operand
	}
	switch value.Scalar {
	case "true", "false":
		operand.Kind, operand.Value = OperandBool, value.Scalar
	case "null":
		operand.Kind = OperandNull
	default:
		if signedNumber(value.Scalar) {
			operand.Kind = OperandNumber
		} else {
			operand.Kind = OperandBare
		}
		operand.Value = value.Scalar
	}
	return operand
}

func cloneOperand(operand *Operand) *Operand {
	if operand == nil {
		return nil
	}
	clone := *operand
	if operand.Items != nil {
		clone.Items = make([]Operand, len(operand.Items))
		for i := range operand.Items {
			clone.Items[i] = *cloneOperand(&operand.Items[i])
		}
	}
	return &clone
}

func operandIdentity(operand *Operand) string {
	var out strings.Builder
	if operand == nil {
		return "N"
	}
	writeOperandIdentity(&out, operand)
	return out.String()
}

func writeOperandIdentity(out *strings.Builder, operand *Operand) {
	out.WriteByte('O')
	writeIdentityString(out, string(operand.Kind))
	writeIdentityString(out, operand.Source)
	writeIdentityString(out, operand.Value)
	out.WriteString(strconv.Itoa(len(operand.Items)))
	out.WriteByte(':')
	for i := range operand.Items {
		writeOperandIdentity(out, &operand.Items[i])
	}
}

func writeIdentityString(out *strings.Builder, s string) {
	out.WriteString(strconv.Itoa(len(s)))
	out.WriteByte(':')
	out.WriteString(s)
}
