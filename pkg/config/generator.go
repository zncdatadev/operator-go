package config

// FileContentGenerator
// we can use this interface to generate file config content in config map
// and use GenerateAllFile function to generate configMap data
type FileContentGenerator interface {
	Generate() string
	FileName() string
}

// EnvGenerator
// we can use this interface to generate env config in config map
// and use GenerateAllEnv function to generate configMap data
type EnvGenerator interface {
	Generate() map[string]string
}

// Parser
// config content parser, can get config content by golang-template or create config content self(customize)
// see template_parser.go
type Parser interface {
	Parse() (string, error)
}

func GenerateAllFile(confGenerator []FileContentGenerator) map[string]string {
	data := make(map[string]string)
	for _, generator := range confGenerator {
		if generator.Generate() != "" {
			data[generator.FileName()] = generator.Generate()
		}
	}
	return data
}

func GenerateAllEnv(confGenerator []EnvGenerator) map[string]string {
	data := make(map[string]string)
	for _, generator := range confGenerator {
		if generator.Generate() != nil {
			for k, v := range generator.Generate() {
				data[k] = v
			}
		}
	}
	return data
}
