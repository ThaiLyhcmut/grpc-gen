package gateway

// ProtoInfo is the parsed representation of one .proto file.
type ProtoInfo struct {
	FilePath    string
	PackageName string  // proto package, e.g. "user"
	GoPackage   string  // option go_package value
	ServiceName string  // e.g. "user" (folder/service identifier)
	Messages    []MessageInfo
	Enums       []EnumInfo
	RPCs        []RPCInfo
}

// MessageInfo is a `message Foo { ... }` block.
type MessageInfo struct {
	Name   string
	Fields []FieldInfo
}

// FieldInfo is one field inside a message.
type FieldInfo struct {
	Name       string // proto name (snake_case)
	Type       string // raw proto type ("string", "int32", "uint64", "google.protobuf.Timestamp", "common.FilterCriteria", ...)
	IsOptional bool
	IsRepeated bool
	IsEnum     bool   // resolved after pass2
	EnumValues []string

	// ModelNullable forces the GraphQL output type nullable (drops the `!`)
	// even when the proto field isn't optional. Set for fields guarded by
	// a field-level @auth/@policy directive: a denied directive must be able
	// to resolve the field to null instead of erroring the whole object.
	ModelNullable bool
}

// EnumInfo is a `enum Status { ... }` block.
type EnumInfo struct {
	Name   string
	Values []string // e.g. ["ACTIVE", "INACTIVE"]
}

// RPCInfo is one `rpc Foo(Bar) returns (Baz)` line in a service definition.
type RPCInfo struct {
	Name         string
	RequestType  string
	ResponseType string
}

// GatewayConfig is the parsed gateway-config.yaml.
type GatewayConfig struct {
	Entities map[string]EntityConfig `yaml:"entities"`

	// ActionAuth is the role required to call the runAction mutation. "*"
	// means any authenticated viewer; empty means no gate.
	ActionAuth string `yaml:"action_auth,omitempty"`
}

// EntityConfig declares the service that owns an entity plus its outbound
// relations to other entities (possibly in other services).
type EntityConfig struct {
	Service   string                    `yaml:"service"`
	Relations map[string]RelationConfig `yaml:"relations,omitempty"`

	// FieldAuth maps a field name → required role. The generated schema
	// puts `@auth(role: "<role>")` on that field.
	FieldAuth map[string]string `yaml:"field_auth,omitempty"`

	// FieldPolicy maps a field name → policy rule key (the Casbin object
	// name). The generated schema puts `@policy(rule: "<key>")` on it.
	FieldPolicy map[string]string `yaml:"field_policy,omitempty"`

	// OpAuth gates the entity's generated operations by role. Keys are
	// create | update | delete | list | get; value is the required role
	// ("*" = any authenticated). Schema gen puts `@auth(role: ...)` on the
	// matching Query/Mutation field.
	OpAuth map[string]string `yaml:"op_auth,omitempty"`
}

// RelationConfig describes a single relation field on an entity.
type RelationConfig struct {
	Target     string `yaml:"target"`               // target entity name
	Kind       string `yaml:"kind"`                 // belongsTo | hasOne | hasMany
	LocalKey   string `yaml:"local_key,omitempty"`  // FK on this entity (for belongsTo)
	ForeignKey string `yaml:"foreign_key,omitempty"` // FK on target entity (for hasOne/hasMany)
}

// IsEntityMessage reports whether the given message is a domain entity (not a
// Create/Update/Delete/List Request/Response).
func IsEntityMessage(name string) bool {
	for _, suffix := range []string{"Request", "Response"} {
		if len(name) > len(suffix) && name[len(name)-len(suffix):] == suffix {
			return false
		}
	}
	return true
}
