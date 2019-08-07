package templates

// Data is the set of properties used by the options template for non-func-
// specific generatopm
type Data struct {
	Now           string
	Package       string
	InstanceName  string
	StructName    string
	StructMembers []FuncData
}

// FuncData is the set of properties used on a per-func basis
type FuncData struct {
	InstanceName    string
	OptionName      string
	OptionNameLower string
	OptionNameUpper string
	OptionType      string
}
