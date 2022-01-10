package world

import (
	"github.com/hajimehoshi/ebiten/v2"
)

type Tile struct {
	Sprite      *ebiten.Image
	HoverSprite *ebiten.Image
}

type GameLevel struct {
	Tiles [][][]*Tile

	size int
}

func NewLevel(size int) *GameLevel {
	l := &GameLevel{
		size: size,
	}
	return l
}

func (l *GameLevel) AddLayer() {
	tileMap := make([][]*Tile, l.size)
	for x := 0; x < l.size; x++ {
		tileMap[x] = make([]*Tile, l.size)
		// TODO
		/*for y := range tileMap[x] {
			tileMap[x][y] = &Tile{
				Sprite: asset.ImgWhiteSquare,
			}
		}*/
	}
	l.Tiles = append(l.Tiles, tileMap)
}

func (l *GameLevel) ClearHoverSprites() {
	for i := range l.Tiles {
		for x := range l.Tiles[i] {
			for _, tile := range l.Tiles[i][x] {
				if tile == nil {
					continue
				}
				tile.HoverSprite = nil
			}
		}
	}
}
