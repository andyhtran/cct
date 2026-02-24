package app

import (
	"encoding/json"
	"os"

	"github.com/alecthomas/kong"
)

type SchemaCmd struct{}

type schemaOutput struct {
	Version     string          `json:"version"`
	GlobalFlags []schemaFlag    `json:"global_flags"`
	Commands    []schemaCommand `json:"commands"`
}

type schemaCommand struct {
	Name     string          `json:"name"`
	Aliases  []string        `json:"aliases,omitempty"`
	Help     string          `json:"help,omitempty"`
	Args     []schemaArg     `json:"args,omitempty"`
	Flags    []schemaFlag    `json:"flags,omitempty"`
	Commands []schemaCommand `json:"commands,omitempty"`
}

type schemaArg struct {
	Name     string `json:"name"`
	Help     string `json:"help,omitempty"`
	Required bool   `json:"required"`
	Type     string `json:"type,omitempty"`
}

type schemaFlag struct {
	Name     string `json:"name"`
	Short    string `json:"short,omitempty"`
	Help     string `json:"help,omitempty"`
	Type     string `json:"type,omitempty"`
	Default  string `json:"default,omitempty"`
	Required bool   `json:"required,omitempty"`
	Enum     string `json:"enum,omitempty"`
	Hidden   bool   `json:"hidden,omitempty"`
}

func (cmd *SchemaCmd) Run(globals *Globals, k *kong.Kong) error {
	out := schemaOutput{
		Version:     appVersion,
		GlobalFlags: extractFlags(k.Model.Flags),
		Commands:    extractCommands(k.Model.Children),
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func extractCommands(nodes []*kong.Node) []schemaCommand {
	var cmds []schemaCommand
	for _, n := range nodes {
		if n.Hidden {
			continue
		}
		cmd := schemaCommand{
			Name:    n.Name,
			Help:    n.Help,
			Aliases: n.Aliases,
		}
		for _, p := range n.Positional {
			cmd.Args = append(cmd.Args, schemaArg{
				Name:     p.Name,
				Help:     p.Help,
				Required: p.Required,
				Type:     p.Tag.Type,
			})
		}
		cmd.Flags = extractFlags(n.Flags)
		cmd.Commands = extractCommands(n.Children)
		cmds = append(cmds, cmd)
	}
	return cmds
}

func extractFlags(flags []*kong.Flag) []schemaFlag {
	var out []schemaFlag
	for _, f := range flags {
		if f.Hidden || f.Name == "help" {
			continue
		}
		sf := schemaFlag{
			Name:     f.Name,
			Help:     f.Help,
			Type:     f.Tag.Type,
			Default:  f.Default,
			Required: f.Required,
			Hidden:   f.Hidden,
		}
		if f.Short != 0 {
			sf.Short = string(f.Short)
		}
		if f.Enum != "" {
			sf.Enum = f.Enum
		}
		if sf.Type == "" && f.Target.IsValid() {
			sf.Type = f.Target.Type().String()
		}
		out = append(out, sf)
	}
	return out
}
