package gateway

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadConfig reads gateway-config.yaml (or returns an empty config if missing).
func LoadConfig(path string) (GatewayConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return GatewayConfig{Entities: map[string]EntityConfig{}}, nil
		}
		return GatewayConfig{}, err
	}
	var cfg GatewayConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse yaml: %w", err)
	}
	if cfg.Entities == nil {
		cfg.Entities = map[string]EntityConfig{}
	}
	return cfg, nil
}

// ValidateConfig checks that every entity & target in the config exists in the
// parsed proto set.
func ValidateConfig(cfg GatewayConfig, protos []ProtoInfo) error {
	known := map[string]bool{}
	for _, p := range protos {
		for _, m := range p.Messages {
			if IsEntityMessage(m.Name) {
				known[m.Name] = true
			}
		}
	}

	for name, ent := range cfg.Entities {
		if !known[name] {
			return fmt.Errorf("entity %q in gateway-config not found in any proto", name)
		}
		for relName, rel := range ent.Relations {
			if !known[rel.Target] {
				return fmt.Errorf("relation %s.%s references unknown target %q", name, relName, rel.Target)
			}
			switch rel.Kind {
			case "belongsTo":
				if rel.LocalKey == "" {
					return fmt.Errorf("relation %s.%s (belongsTo) needs local_key", name, relName)
				}
			case "hasOne", "hasMany":
				if rel.ForeignKey == "" {
					return fmt.Errorf("relation %s.%s (%s) needs foreign_key", name, relName, rel.Kind)
				}
			default:
				return fmt.Errorf("relation %s.%s has unknown kind %q (use belongsTo|hasOne|hasMany)", name, relName, rel.Kind)
			}
		}

		// Field-level @auth / @policy must name real fields on the entity.
		fields := map[string]bool{}
		for _, p := range protos {
			for _, m := range p.Messages {
				if m.Name == name {
					for _, f := range m.Fields {
						fields[f.Name] = true
					}
				}
			}
		}
		for fld := range ent.FieldAuth {
			if !fields[fld] {
				return fmt.Errorf("field_auth: %s has no field %q", name, fld)
			}
		}
		for fld := range ent.FieldPolicy {
			if !fields[fld] {
				return fmt.Errorf("field_policy: %s has no field %q", name, fld)
			}
		}

		// op_auth keys must be one of the generated operations.
		validOps := map[string]bool{
			"create": true, "update": true, "delete": true, "list": true, "get": true,
		}
		for op := range ent.OpAuth {
			if !validOps[op] {
				return fmt.Errorf("op_auth: %s has unknown operation %q (use create|update|delete|list|get)", name, op)
			}
		}
	}
	return nil
}

// ApplyFieldDirectives marks every field named in a field_auth / field_policy
// entry as ModelNullable, so schema gen drops the `!` and converter gen
// pointer-wraps it. Call after ValidateConfig. Mutates protos in place.
func ApplyFieldDirectives(cfg GatewayConfig, protos []ProtoInfo) {
	for entName, ent := range cfg.Entities {
		guarded := map[string]bool{}
		for fld := range ent.FieldAuth {
			guarded[fld] = true
		}
		for fld := range ent.FieldPolicy {
			guarded[fld] = true
		}
		if len(guarded) == 0 {
			continue
		}
		for pi := range protos {
			for mi := range protos[pi].Messages {
				if protos[pi].Messages[mi].Name != entName {
					continue
				}
				for fi := range protos[pi].Messages[mi].Fields {
					f := &protos[pi].Messages[mi].Fields[fi]
					if guarded[f.Name] {
						f.ModelNullable = true
					}
				}
			}
		}
	}
}
