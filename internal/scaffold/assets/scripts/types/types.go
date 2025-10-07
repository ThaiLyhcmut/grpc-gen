package types

type Data struct {
	PackagePath string
	ProtoName   string
	ServiceName string
	Port        string
	ModulePath  string
}

type HandlerData struct {
	PackagePath string
	ServiceName string
	ModulePath  string
}

type Method struct {
	Name         string
	RequestType  string
	ResponseType string
}

type EntityHandlerData struct {
	PackagePath string
	EntityName  string
	Methods     []Method
}

type Field struct {
	Name           string
	ProtoName      string
	Type           string
	GoName         string
	DBField        string
	EnumType       string
	EnumValues     []string
	DefaultValue   string
	DefaultDBValue string
	IsOptional     bool
	IsEnum         bool
	IsTimestamp    bool
}

type CRUDHandlerData struct {
	PackagePath        string
	EntityName         string
	TableName          string
	Methods            []Method
	EnumType           string
	RequiredFields     []Field
	OptionalFields     []Field
	EnumFields         []Field
	FilterableFields   []string
	CreateFields       []Field
	CreateFieldsSQL    string
	CreatePlaceholders string
	UpdateFields       []Field
	SelectFieldsSQL    string
	ScanFields         []string
}
