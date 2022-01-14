package world

import (
	"errors"
	"fmt"
	"image"
	"log"
	"math/rand"
	"path/filepath"

	"code.rocketnine.space/tslocum/citylimits/asset"
	"code.rocketnine.space/tslocum/citylimits/component"
	. "code.rocketnine.space/tslocum/citylimits/ecs"
	"code.rocketnine.space/tslocum/gohan"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/lafriks/go-tiled"
)

const TileSize = 64

var DirtTile = uint32(9*32 + (0))

const (
	StructureBulldozer = iota + 1
	StructureRoad
	StructureHouse1
	StructureBusiness1
	StructurePoliceStation
)

const startingZoom = 1.0

const SidebarWidth = 198

type HUDButton struct {
	Sprite                       *ebiten.Image
	SpriteOffsetX, SpriteOffsetY float64
	Label                        string
	StructureType                int
}

var HUDButtons []*HUDButton

var World = &GameWorld{
	CamScale:       startingZoom,
	CamScaleTarget: startingZoom,
	CamMoving:      true,
	PlayerWidth:    8,
	PlayerHeight:   32,
	TileImages:     make(map[uint32]*ebiten.Image),
	ResetGame:      true,
	Level:          NewLevel(256),
}

type GameWorld struct {
	Level *GameLevel

	Player gohan.Entity

	ScreenW, ScreenH int

	DisableEsc bool

	Debug  int
	NoClip bool

	GameStarted      bool
	GameStartedTicks int
	GameOver         bool

	MessageVisible  bool
	MessageTicks    int
	MessageDuration int
	MessageUpdated  bool
	MessageText     string

	PlayerX, PlayerY float64

	CamX, CamY     float64
	CamScale       float64
	CamScaleTarget float64
	CamMoving      bool

	PlayerWidth  float64
	PlayerHeight float64

	HoverStructure         int
	HoverX, HoverY         int
	HoverLastX, HoverLastY int
	HoverValid             bool

	Map             *tiled.Map
	ObjectGroups    []*tiled.ObjectGroup
	HazardRects     []image.Rectangle
	CreepRects      []image.Rectangle
	CreepEntities   []gohan.Entity
	TriggerEntities []gohan.Entity
	TriggerRects    []image.Rectangle
	TriggerNames    []string

	NativeResolution bool

	BrokenPieceA, BrokenPieceB gohan.Entity

	TileImages         map[uint32]*ebiten.Image
	TileImagesFirstGID uint32

	ResetGame bool

	MuteMusic        bool
	MuteSoundEffects bool // TODO

	GotCursorPosition bool

	tilesets []*ebiten.Image

	EnvironmentSprites int

	SelectedStructure *Structure

	HUDUpdated     bool
	HUDButtonRects []image.Rectangle

	resetTipShown bool
}

func TileToGameCoords(x, y int) (float64, float64) {
	//return float64(x) * 32, float64(g.currentMap.Height*32) - float64(y)*32 - 32
	return float64(x) * TileSize, float64(y) * TileSize
}

func Reset() {
	for _, e := range ECS.Entities() {
		ECS.RemoveEntity(e)
	}
	World.Player = 0

	World.ObjectGroups = nil
	World.HazardRects = nil
	World.CreepRects = nil
	World.CreepEntities = nil
	World.TriggerEntities = nil
	World.TriggerRects = nil
	World.TriggerNames = nil

	World.MessageVisible = false
}

func LoadMap(structureType int) (*tiled.Map, error) {
	loader := tiled.Loader{
		FileSystem: asset.FS,
	}

	var filePath string
	switch structureType {
	case StructureBulldozer:
		filePath = "map/bulldozer.tmx"
	case StructureRoad:
		filePath = "map/road.tmx"
	case StructureHouse1:
		filePath = "map/house1.tmx"
	case StructureBusiness1:
		filePath = "map/business1.tmx"
	case StructurePoliceStation:
		filePath = "map/policestation.tmx"
	default:
		panic(fmt.Sprintf("unknown structure %d", structureType))
	}

	// Parse .tmx file.
	m, err := loader.LoadFromFile(filepath.FromSlash(filePath))
	if err != nil {
		log.Fatalf("error parsing world: %+v", err)
	}

	return m, err
}

func DrawMap(structureType int) *ebiten.Image {
	img := ebiten.NewImage(128, 128)

	m, err := LoadMap(structureType)
	if err != nil {
		panic(err)
	}

	var t *tiled.LayerTile
	for i, layer := range m.Layers {
		for y := 0; y < m.Height; y++ {
			for x := 0; x < m.Width; x++ {
				t = layer.Tiles[y*m.Width+x]
				if t == nil || t.Nil {
					continue // No tile at this position.
				}

				tileImg := World.TileImages[t.Tileset.FirstGID+t.ID]
				if tileImg == nil {
					continue
				}

				xi, yi := CartesianToIso(float64(x), float64(y))

				scale := 0.9 / float64(m.Width)
				if m.Width < 2 {
					scale = 0.6
				}

				paddingX := 64.0
				op := &ebiten.DrawImageOptions{}
				op.GeoM.Translate(xi+(paddingX*(float64(m.Width)-1)), (yi+float64(i*-40))+92)
				op.GeoM.Scale(scale, scale)
				img.DrawImage(tileImg, op)
			}
		}
	}

	return img
}

func LoadTileset() error {
	m, err := LoadMap(StructureHouse1)
	if err != nil {
		return err
	}

	// Load tileset.

	if len(World.tilesets) != 0 {
		return nil // Already loaded.
	}

	tileset := m.Tilesets[0]
	imgPath := filepath.Join("./image/tileset/", tileset.Image.Source)
	f, err := asset.FS.Open(filepath.FromSlash(imgPath))
	if err != nil {
		panic(err)
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		panic(err)
	}
	World.tilesets = append(World.tilesets, ebiten.NewImageFromImage(img))

	// Load tiles.

	for i := uint32(0); i < uint32(tileset.TileCount); i++ {
		rect := tileset.GetTileRect(i)
		World.TileImages[i+tileset.FirstGID] = World.tilesets[0].SubImage(rect).(*ebiten.Image)
	}

	World.TileImagesFirstGID = tileset.FirstGID
	return nil
}

func BuildStructure(structureType int, hover bool, placeX int, placeY int) (*Structure, error) {
	m, err := LoadMap(structureType)
	if err != nil {
		return nil, err
	}

	w := m.Width - 1
	h := m.Height - 1

	if placeX-w < 0 || placeY-h < 0 || placeX > 256 || placeY > 256 {
		return nil, errors.New("invalid location: building does not fit")
	}

	createTileEntity := func(t *tiled.LayerTile, x float64, y float64) gohan.Entity {
		mapTile := ECS.NewEntity()
		ECS.AddComponent(mapTile, &component.PositionComponent{
			X: x,
			Y: y,
		})

		sprite := &component.SpriteComponent{
			Image:          World.TileImages[t.Tileset.FirstGID+t.ID],
			HorizontalFlip: t.HorizontalFlip,
			VerticalFlip:   t.VerticalFlip,
			DiagonalFlip:   t.DiagonalFlip,
		}
		ECS.AddComponent(mapTile, sprite)

		return mapTile
	}
	_ = createTileEntity

	structure := &Structure{
		Type: structureType,
		X:    placeX,
		Y:    placeY,
	}

	// TODO Add entity

	valid := true
	var existingRoadTiles int
VALIDBUILD:
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			tx, ty := (x+placeX)-w, (y+placeY)-h
			if structureType == StructureRoad && World.Level.Tiles[0][tx][ty].Sprite == World.TileImages[World.TileImagesFirstGID] {
				existingRoadTiles++
			}
			if World.Level.Tiles[1][tx][ty].Sprite != nil || (World.Level.Tiles[0][tx][ty].Sprite != nil && (structureType != StructureRoad || World.Level.Tiles[0][tx][ty].Sprite != World.TileImages[World.TileImagesFirstGID])) {
				valid = false
				break VALIDBUILD
			}
		}
	}
	if structureType == StructureRoad && existingRoadTiles == 4 {
		valid = false
	}
	if hover {
		World.HoverValid = valid
	} else if !valid {
		return nil, errors.New("invalid location: space already occupied")
	}

	World.Level.ClearHoverSprites()

	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			tx, ty := (x+placeX)-w, (y+placeY)-h
			if hover {
				World.Level.Tiles[0][tx][ty].HoverSprite = World.TileImages[World.TileImagesFirstGID]
				if valid {
					// Hide environment sprites temporarily.
					for i := 1; i < len(World.Level.Tiles); i++ {
						World.Level.Tiles[i][tx][ty].HoverSprite = asset.ImgBlank
					}
				}
			} else {
				World.Level.Tiles[0][tx][ty].Sprite = World.TileImages[World.TileImagesFirstGID]
				World.Level.Tiles[0][tx][ty].EnvironmentSprite = nil
				World.Level.Tiles[1][tx][ty].EnvironmentSprite = nil
			}
		}
	}

	var t *tiled.LayerTile
	for i, layer := range m.Layers {
		for y := 0; y < m.Height; y++ {
			for x := 0; x < m.Width; x++ {
				t = layer.Tiles[y*m.Width+x]
				if t == nil || t.Nil {
					continue // No tile at this position.
				}

				tileImg := World.TileImages[t.Tileset.FirstGID+t.ID]
				if tileImg == nil {
					continue
				}

				layerNum := i
				if structureType != StructureRoad {
					layerNum++
				}

				for layerNum > len(World.Level.Tiles)-1 {
					World.Level.AddLayer()
				}

				tx, ty := (x+placeX)-w, (y+placeY)-h
				if hover {
					World.Level.Tiles[layerNum][tx][ty].HoverSprite = World.TileImages[t.Tileset.FirstGID+t.ID]
				} else {
					World.Level.Tiles[layerNum][tx][ty].Sprite = World.TileImages[t.Tileset.FirstGID+t.ID]
				}

				// TODO handle flipping
			}
		}
	}

	return structure, nil
}

func ObjectToRect(o *tiled.Object) image.Rectangle {
	x, y, w, h := int(o.X), int(o.Y), int(o.Width), int(o.Height)
	y -= 32
	return image.Rect(x, y, x+w, y+h)
}

func LevelCoordinatesToScreen(x, y float64) (float64, float64) {
	return (x - World.CamX) * World.CamScale, (y - World.CamY) * World.CamScale
}

func (w *GameWorld) SetGameOver(vx, vy float64) {
	if w.GameOver {
		return
	}

	w.GameOver = true

	// TODO
}

// TODO move
func NewActor(creepType int, creepID int64, x float64, y float64) gohan.Entity {
	actor := ECS.NewEntity()

	ECS.AddComponent(actor, &component.PositionComponent{
		X: x,
		Y: y,
	})

	ECS.AddComponent(actor, &component.ActorComponent{
		Type:       creepType,
		Health:     64,
		FireAmount: 8,
		FireRate:   144 / 4,
		Rand:       rand.New(rand.NewSource(creepID)),
	})

	return actor
}

func StartGame() {
	if World.GameStarted {
		return
	}
	World.GameStarted = true

	if !World.MuteMusic {
		asset.SoundMusic.Play()
	}
}

func SetMessage(message string, duration int) {
	World.MessageText = message
	World.MessageVisible = true
	World.MessageUpdated = true
	World.MessageDuration = duration
	World.MessageTicks = 0
}

// CartesianToIso transforms cartesian coordinates into isometric coordinates.
func CartesianToIso(x, y float64) (float64, float64) {
	ix := (x - y) * float64(TileSize/2)
	iy := (x + y) * float64(TileSize/4)
	return ix, iy
}

// CartesianToIso transforms cartesian coordinates into isometric coordinates.
func IsoToCartesian(x, y float64) (float64, float64) {
	cx := (x/float64(TileSize/2) + y/float64(TileSize/4)) / 2
	cy := (y/float64(TileSize/4) - (x / float64(TileSize/2))) / 2
	cx-- // TODO Why is this necessary?
	return cx, cy
}

func IsoToScreen(x, y float64) (float64, float64) {
	cx, cy := float64(World.ScreenW/2), float64(World.ScreenH/2)
	return ((x - World.CamX) * World.CamScale) + cx, ((y - World.CamY) * World.CamScale) + cy
}

func ScreenToIso(x, y int) (float64, float64) {
	// Offset cursor to first above ground layer.
	y += int(float64(16) * World.CamScale)

	cx, cy := float64(World.ScreenW/2), float64(World.ScreenH/2)
	return ((float64(x) - cx) / World.CamScale) + World.CamX, ((float64(y) - cy) / World.CamScale) + World.CamY
}

func ScreenToCartesian(x, y int) (float64, float64) {
	xi, yi := ScreenToIso(x, y)
	return IsoToCartesian(xi, yi)
}

func HUDButtonAt(x, y int) *HUDButton {
	for i, colorRect := range World.HUDButtonRects {
		point := image.Point{x, y}
		if point.In(colorRect) {
			return HUDButtons[i]
		}
	}
	return nil
}

func SetHoverStructure(structureType int) {
	World.HoverStructure = structureType
	World.HUDUpdated = true
}

func Demand() (r, c, i float64) {
	r = (rand.Float64() * 2) - 1
	c = (rand.Float64() * 2) - 1
	i = (rand.Float64() * 2) - 1
	return r, c, i
}

var tooltips = map[int]string{
	StructureBulldozer:     "Bulldoze",
	StructureRoad:          "Road",
	StructureHouse1:        "Residential",
	StructureBusiness1:     "Commercial",
	StructurePoliceStation: "Police station",
}

func Tooltip() string {
	return tooltips[World.HoverStructure]
}
