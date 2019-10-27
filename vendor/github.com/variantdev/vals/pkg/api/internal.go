package api

// StaticConfig is pre-loaded configuration that has zero changes to fail at the time of getting values.
// Examples of StaticConfig:
//
// values:
// - provider:
//     type: string
//     name: ssm
//     prefix: myteam/mysvc1
//     region: ap-northeast-1
//   inline:
//     dst1: src1
//
// values:
// - provider:
//     type: map
//     name: ssm
//     prefix: myteam/mysvc1
//     region: ap-northeast-1
//     strategy: raw
//   inline:
//     dst1: src1
//
// values:
// - provider:
//     type: map
//     name: ssm
//     prefix: myteam/mysvc1
//     region: ap-northeast-1
//     strategy: yaml
//   inline:
//     dst1: src1
//
// values:
// - provider:
//     type: string
//     name: vault
//     address: http://127.0.0.1:8200
//     path: secrets/myteam/mysvc1
//   inline:
//     dst1: src1
//
// values:
// - vault:
//     address: http://127.0.0.1:8200
//     path: secrets/myteam/mysvc1
//   inline:
//     dst1: src1
//
type StaticConfig interface {
	String(...string) string
	Config(...string) StaticConfig
	Exists(...string) bool
	Map(...string) map[string]interface{}
	StringSlice(...string) []string
}

// LazyLoadedStringMapProvider is a variant of string-map providers that connects to/loads the source at the time of getting a value
type LazyLoadedStringMapProvider interface {
	GetStringMap(string) (map[string]interface{}, error)
}

// LazyLoadedStringProvider is a variant of value providers that connects to/loads the source at the time of getting a value
type LazyLoadedStringProvider interface {
	GetString(string) (string, error)
}

type Provider interface {
	LazyLoadedStringProvider
	LazyLoadedStringMapProvider
}

type Merger interface {
	Merge(map[string]interface{}, map[string]interface{}) (map[string]interface{}, error)
	IgnorePrefix() string
}
