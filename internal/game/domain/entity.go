package domain

type Player struct {
	ID      int64
	Name    string
	SceneID int64
	X       int32
	Y       int32
}

type Monster struct {
	ID      int64
	Name    string
	SceneID int64
	MaxHP   int32
	HP      int32
	X       int32
	Y       int32
	Dead    bool
}
