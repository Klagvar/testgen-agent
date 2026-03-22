package analyzer

import "strings"

// FindImplementors returns concrete types that implement the given interface.
// A type implements an interface if it has all the interface's methods.
// Checks both pointer and value receiver methods.
func FindImplementors(iface TypeInfo, allTypes []TypeInfo, allFuncs []FuncInfo) []TypeInfo {
	if iface.Kind != "interface" || len(iface.Methods) == 0 {
		return nil
	}

	typeMethods := make(map[string]map[string]string) // typeName → methodName → signature

	for _, fn := range allFuncs {
		if fn.Receiver == "" {
			continue
		}
		recvName := strings.TrimPrefix(fn.Receiver, "*")
		if typeMethods[recvName] == nil {
			typeMethods[recvName] = make(map[string]string)
		}
		typeMethods[recvName][fn.Name] = buildMethodSig(fn)
	}

	var implementors []TypeInfo
	for _, ti := range allTypes {
		if ti.Kind == "interface" || ti.Name == iface.Name {
			continue
		}
		methods := typeMethods[ti.Name]
		if methods == nil {
			continue
		}
		if satisfies(iface.Methods, methods) {
			implementors = append(implementors, ti)
		}
	}
	return implementors
}

func buildMethodSig(fn FuncInfo) string {
	var sb strings.Builder
	sb.WriteString("(")
	for i, p := range fn.Params {
		if i > 0 {
			sb.WriteString(", ")
		}
		if p.Name != "" {
			sb.WriteString(p.Name + " ")
		}
		sb.WriteString(p.Type)
	}
	sb.WriteString(")")
	if len(fn.Returns) > 0 {
		sb.WriteString(" ")
		if len(fn.Returns) == 1 {
			sb.WriteString(fn.Returns[0])
		} else {
			sb.WriteString("(" + strings.Join(fn.Returns, ", ") + ")")
		}
	}
	return sb.String()
}

// satisfies checks if the method set covers all interface methods.
// Compares by method name only (signature matching is too strict
// because parameter names may differ).
func satisfies(ifaceMethods []MethodInfo, typeMethods map[string]string) bool {
	for _, m := range ifaceMethods {
		if _, ok := typeMethods[m.Name]; !ok {
			return false
		}
	}
	return true
}
