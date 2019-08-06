package templates

type Data struct {
	Package       string
	InstanceName  string
	StructName    string
	StructMembers []FuncData
}

type FuncData struct {
	InstanceName    string
	OptionName      string
	OptionNameLower string
	OptionNameUpper string
	OptionType      string
}
