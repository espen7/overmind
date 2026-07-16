package home

//go:generate go run ../../../tools/modgen/main.go -input section_def.go -output section_gen.go

// modgen:section key=profile
type ProfileSection struct {
	dirty      uint64 `json:"-"`
	Name       string `json:"name" mod:"getter,setter"`
	Gold       int32  `json:"gold" mod:"getter,setter,add"`
	Power      int32  `json:"power" mod:"getter,setter,add"`
	BuildLevel int32  `json:"build_level" mod:"getter,setter"`
}

// modgen:section key=bag
type BagSection struct {
	dirty uint64          `json:"-"`
	Items map[int32]int32 `json:"items" mod:"getter,setter,map_get,map_set,map_del"`
}

// modgen:section key=builds
type BuildsSection struct {
	dirty  uint64           `json:"-"`
	Builds map[string]int32 `json:"builds" mod:"getter,setter,map_get,map_set,map_del"`
}
