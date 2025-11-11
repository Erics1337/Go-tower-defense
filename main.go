package main

import (
	"fmt"
	"image/color"
	"math"
	"math/rand"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/examples/resources/fonts"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

const (
	screenWidth  = 1024
	screenHeight = 768

	pathWidth   = 52
	towerRadius = 28
	enemyRadius = 18

	towerCost       = 80
	upgradeCost     = 120
	startingGold    = 180
	startingLives   = 20
	projectileSpeed = 420
)

type vec2 struct {
	X float64
	Y float64
}

type enemy struct {
	id        int
	progress  float64
	speed     float64
	health    float64
	maxHealth float64
	reward    int
	alive     bool
	pos       vec2
}

type tower struct {
	pos       vec2
	rangePx   float64
	cooldown  float64
	fireTimer float64
	level     int
	color     color.RGBA
}

type projectile struct {
	pos      vec2
	velocity vec2
	damage   float64
	targetID int
	color    color.RGBA
	alive    bool
}

type wave struct {
	count         int
	spawnInterval float64
	speed         float64
	health        float64
	reward        int
}

type sparkle struct {
	base  vec2
	amp   float64
	size  float32
	phase float64
}

type game struct {
	path        []vec2
	pathLengths []float64
	totalLength float64

	background *ebiten.Image

	enemies     []*enemy
	towers      []*tower
	projectiles []*projectile

	sparkles []sparkle

	nextEnemyID    int
	spawnTimer     float64
	enemiesSpawned int
	waveIndex      int
	waves          []wave

	gold  int
	lives int
	score int

	hoverValid bool
	shimmer    float64

	lastUpdate time.Time
}

func newGame() *game {
	g := &game{}
	g.path = []vec2{
		{100, -40},
		{100, 160},
		{260, 160},
		{260, 340},
		{500, 340},
		{500, 160},
		{720, 160},
		{720, 520},
		{880, 520},
		{880, 820},
	}
	g.computePathLengths()

	g.background = createBackground()

	g.enemies = []*enemy{}
	g.towers = []*tower{}
	g.projectiles = []*projectile{}

	g.gold = startingGold
	g.lives = startingLives

	g.waves = []wave{
		{count: 6, spawnInterval: 0.85, speed: 80, health: 80, reward: 18},
		{count: 8, spawnInterval: 0.75, speed: 100, health: 110, reward: 22},
		{count: 10, spawnInterval: 0.65, speed: 110, health: 150, reward: 25},
		{count: 12, spawnInterval: 0.6, speed: 120, health: 190, reward: 28},
		{count: 1, spawnInterval: 0.0, speed: 70, health: 1200, reward: 120},
	}

	rand.Seed(time.Now().UnixNano())
	for i := 0; i < 26; i++ {
		g.sparkles = append(g.sparkles, sparkle{
			base:  vec2{rand.Float64() * screenWidth, rand.Float64() * screenHeight},
			amp:   16 + rand.Float64()*28,
			size:  float32(3 + rand.Float64()*9),
			phase: rand.Float64() * math.Pi * 2,
		})
	}

	g.lastUpdate = time.Now()
	return g
}

func (g *game) computePathLengths() {
	g.pathLengths = make([]float64, len(g.path)-1)
	g.totalLength = 0
	for i := 0; i < len(g.path)-1; i++ {
		seg := distance(g.path[i], g.path[i+1])
		g.pathLengths[i] = seg
		g.totalLength += seg
	}
}

func (g *game) Update() error {
	now := time.Now()
	dt := now.Sub(g.lastUpdate).Seconds()
	if dt > 0.1 {
		dt = 0.1
	}
	g.lastUpdate = now

	if g.lives <= 0 {
		if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
			*g = *newGame()
		}
		return nil
	}

	g.shimmer += dt

	g.handleInput()
	g.spawnEnemies(dt)
	g.updateEnemies(dt)
	g.updateTowers(dt)
	g.updateProjectiles(dt)
	g.cleanup()

	return nil
}

func (g *game) spawnEnemies(dt float64) {
	if g.waveIndex >= len(g.waves) {
		return
	}

	w := g.waves[g.waveIndex]
	if g.enemiesSpawned >= w.count {
		if len(g.enemies) == 0 {
			g.waveIndex++
			g.enemiesSpawned = 0
			g.spawnTimer = 0
		}
		return
	}

	g.spawnTimer -= dt
	if g.spawnTimer <= 0 {
		g.spawnTimer = w.spawnInterval
		e := &enemy{
			id:        g.nextEnemyID,
			progress:  0,
			speed:     w.speed,
			health:    w.health,
			maxHealth: w.health,
			reward:    w.reward,
			alive:     true,
		}
		g.nextEnemyID++
		e.pos = g.positionAlongPath(0)
		g.enemies = append(g.enemies, e)
		g.enemiesSpawned++
	}
}

func (g *game) updateEnemies(dt float64) {
	for _, e := range g.enemies {
		if !e.alive {
			continue
		}
		e.progress += e.speed * dt
		if e.progress >= g.totalLength {
			e.alive = false
			g.lives--
			if g.lives < 0 {
				g.lives = 0
			}
			continue
		}
		e.pos = g.positionAlongPath(e.progress)
	}
}

func (g *game) updateTowers(dt float64) {
	cursorX, cursorY := ebiten.CursorPosition()
	cursorPos := vec2{float64(cursorX), float64(cursorY)}
	g.hoverValid = g.canPlaceTower(cursorPos)

	for _, t := range g.towers {
		t.fireTimer -= dt
		if t.fireTimer > 0 {
			continue
		}
		target := g.findTarget(t)
		if target == nil {
			continue
		}
		t.fireTimer = t.cooldown
		g.fireProjectile(t, target)
	}
}

func (g *game) findTarget(t *tower) *enemy {
	var chosen *enemy
	bestProgress := -1.0
	for _, e := range g.enemies {
		if !e.alive {
			continue
		}
		d := distance(t.pos, e.pos)
		if d > t.rangePx {
			continue
		}
		if e.progress > bestProgress {
			chosen = e
			bestProgress = e.progress
		}
	}
	return chosen
}

func (g *game) fireProjectile(t *tower, e *enemy) {
	dir := vec2{e.pos.X - t.pos.X, e.pos.Y - t.pos.Y}
	mag := math.Hypot(dir.X, dir.Y)
	if mag == 0 {
		return
	}
	dir.X /= mag
	dir.Y /= mag
	speed := projectileSpeed + rand.Float64()*80
	proj := &projectile{
		pos:      vec2{t.pos.X, t.pos.Y},
		velocity: vec2{dir.X * speed, dir.Y * speed},
		damage:   25 + float64(t.level)*12,
		targetID: e.id,
		alive:    true,
		color:    color.RGBA{R: 255, G: uint8(170 + 15*t.level), B: 140, A: 230},
	}
	g.projectiles = append(g.projectiles, proj)
}

func (g *game) updateProjectiles(dt float64) {
	for _, p := range g.projectiles {
		if !p.alive {
			continue
		}
		p.pos.X += p.velocity.X * dt
		p.pos.Y += p.velocity.Y * dt
		for _, e := range g.enemies {
			if !e.alive || e.id != p.targetID {
				continue
			}
			if distance(p.pos, e.pos) <= enemyRadius {
				e.health -= p.damage
				if e.health <= 0 {
					e.alive = false
					g.gold += e.reward
					g.score += e.reward * 3
				}
				p.alive = false
				break
			}
		}
		if p.pos.X < -60 || p.pos.Y < -60 || p.pos.X > screenWidth+60 || p.pos.Y > screenHeight+60 {
			p.alive = false
		}
	}
}

func (g *game) cleanup() {
	enemies := g.enemies[:0]
	for _, e := range g.enemies {
		if e.alive {
			enemies = append(enemies, e)
		}
	}
	g.enemies = enemies

	projectiles := g.projectiles[:0]
	for _, p := range g.projectiles {
		if p.alive {
			projectiles = append(projectiles, p)
		}
	}
	g.projectiles = projectiles
}

func (g *game) handleInput() {
	cursorX, cursorY := ebiten.CursorPosition()
	pos := vec2{float64(cursorX), float64(cursorY)}

	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		if g.gold >= towerCost && g.canPlaceTower(pos) {
			g.placeTower(pos)
			g.gold -= towerCost
		}
	}

	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
		g.upgradeTower(pos)
	}
}

func (g *game) placeTower(pos vec2) {
	col := vibrantColor(len(g.towers))
	t := &tower{
		pos:      pos,
		rangePx:  200,
		cooldown: 0.55,
		level:    1,
		color:    col,
	}
	g.towers = append(g.towers, t)
}

func (g *game) upgradeTower(pos vec2) {
	var closest *tower
	bestDist := 9999.0
	for _, t := range g.towers {
		d := distance(pos, t.pos)
		if d < towerRadius && d < bestDist {
			closest = t
			bestDist = d
		}
	}
	if closest != nil && g.gold >= upgradeCost {
		g.gold -= upgradeCost
		closest.level++
		closest.rangePx += 24
		closest.cooldown *= 0.88
		closest.color = upgradeColor(closest.level)
	}
}

func upgradeColor(level int) color.RGBA {
	palette := []color.RGBA{
		{R: 252, G: 208, B: 35, A: 255},
		{R: 96, G: 217, B: 231, A: 255},
		{R: 180, G: 120, B: 255, A: 255},
		{R: 255, G: 108, B: 181, A: 255},
	}
	return palette[(level-1)%len(palette)]
}

func (g *game) canPlaceTower(pos vec2) bool {
	if pos.X < towerRadius || pos.Y < towerRadius || pos.X > screenWidth-towerRadius || pos.Y > screenHeight-towerRadius {
		return false
	}

	for i := 0; i < len(g.path)-1; i++ {
		d := distancePointToSegment(pos, g.path[i], g.path[i+1])
		if d < pathWidth/2+towerRadius-6 {
			return false
		}
	}

	for _, t := range g.towers {
		if distance(pos, t.pos) < towerRadius*2.1 {
			return false
		}
	}

	return true
}

func (g *game) positionAlongPath(progress float64) vec2 {
	remaining := progress
	for i := 0; i < len(g.pathLengths); i++ {
		segLen := g.pathLengths[i]
		if remaining <= segLen {
			ratio := remaining / segLen
			start := g.path[i]
			end := g.path[i+1]
			return vec2{
				X: start.X + (end.X-start.X)*ratio,
				Y: start.Y + (end.Y-start.Y)*ratio,
			}
		}
		remaining -= segLen
	}
	return g.path[len(g.path)-1]
}

func (g *game) Draw(screen *ebiten.Image) {
	screen.DrawImage(g.background, nil)

	g.drawPath(screen)
	g.drawSparkles(screen)
	g.drawTowers(screen)
	g.drawEnemies(screen)
	g.drawProjectiles(screen)
	g.drawUI(screen)

	if g.lives <= 0 {
		g.drawGameOver(screen)
	}
}

func (g *game) drawGameOver(screen *ebiten.Image) {
	msg := fmt.Sprintf("Defenses overwhelmed! Score: %d", g.score)
	drawCenteredText(screen, msg, screenWidth/2, screenHeight/2-20, color.RGBA{255, 240, 220, 255}, 1.6)
	drawCenteredText(screen, "Press SPACE to relaunch", screenWidth/2, screenHeight/2+24, color.RGBA{230, 200, 250, 255}, 1.2)
}

func (g *game) drawPath(screen *ebiten.Image) {
	var base vector.Path
	start := g.path[0]
	base.MoveTo(float32(start.X), float32(start.Y))
	for i := 1; i < len(g.path); i++ {
		next := g.path[i]
		base.LineTo(float32(next.X), float32(next.Y))
	}

	glowAlpha := uint8(65 + 40*math.Sin(g.shimmer*1.3))
	glowColor := color.RGBA{R: 132, G: 94, B: 247, A: glowAlpha}
	strokeColor := color.RGBA{R: 60, G: 50, B: 85, A: 240}

	var glowScale ebiten.ColorScale
	glowScale.Scale(float32(glowColor.R)/255, float32(glowColor.G)/255, float32(glowColor.B)/255, float32(glowColor.A)/255)
	vector.StrokePath(screen, &base, &vector.StrokeOptions{Width: pathWidth * 1.6, LineCap: vector.LineCapRound, LineJoin: vector.LineJoinRound}, &vector.DrawPathOptions{AntiAlias: true, ColorScale: glowScale})

	var coreScale ebiten.ColorScale
	coreScale.Scale(float32(strokeColor.R)/255, float32(strokeColor.G)/255, float32(strokeColor.B)/255, float32(strokeColor.A)/255)
	vector.StrokePath(screen, &base, &vector.StrokeOptions{Width: pathWidth, LineCap: vector.LineCapRound, LineJoin: vector.LineJoinRound}, &vector.DrawPathOptions{AntiAlias: true, ColorScale: coreScale})
}

func (g *game) drawSparkles(screen *ebiten.Image) {
	for i := range g.sparkles {
		s := &g.sparkles[i]
		offset := math.Sin(g.shimmer*1.5+s.phase) * s.amp
		x := float32(s.base.X + math.Sin(g.shimmer+s.phase)*offset)
		y := float32(s.base.Y + math.Cos(g.shimmer*0.8+s.phase)*offset)
		alpha := uint8(80 + 100*math.Abs(math.Sin(g.shimmer*2+s.phase)))
		vector.DrawFilledCircle(screen, x, y, s.size, color.RGBA{R: 255, G: 255, B: 255, A: alpha}, false)
	}
}

func (g *game) drawTowers(screen *ebiten.Image) {
	for _, t := range g.towers {
		rangeColor := color.RGBA{R: t.color.R, G: t.color.G, B: t.color.B, A: 36}
		vector.DrawFilledCircle(screen, float32(t.pos.X), float32(t.pos.Y), float32(t.rangePx), rangeColor, false)
		vector.DrawFilledCircle(screen, float32(t.pos.X), float32(t.pos.Y), towerRadius, t.color, false)
	}

	cursorX, cursorY := ebiten.CursorPosition()
	cursorPos := vec2{float64(cursorX), float64(cursorY)}
	if g.gold >= towerCost {
		col := color.RGBA{R: 120, G: 255, B: 210, A: 90}
		if !g.hoverValid {
			col = color.RGBA{R: 255, G: 120, B: 120, A: 120}
		}
		vector.DrawFilledCircle(screen, float32(cursorPos.X), float32(cursorPos.Y), towerRadius, col, false)
	}
}

func (g *game) drawEnemies(screen *ebiten.Image) {
	for _, e := range g.enemies {
		if !e.alive {
			continue
		}
		pulse := 0.8 + 0.2*math.Sin(g.shimmer*3+float64(e.id))
		bodyColor := color.RGBA{R: uint8(200 * pulse), G: 70, B: 200, A: 255}
		vector.DrawFilledCircle(screen, float32(e.pos.X), float32(e.pos.Y), enemyRadius, bodyColor, false)

		hpRatio := math.Max(e.health/e.maxHealth, 0)
		barWidth := float32(44)
		barHeight := float32(7)
		x := float32(e.pos.X) - barWidth/2
		y := float32(e.pos.Y) - float32(enemyRadius) - 14
		vector.DrawFilledRect(screen, x, y, barWidth, barHeight, color.RGBA{0, 0, 0, 120}, false)
		vector.DrawFilledRect(screen, x+1, y+1, (barWidth-2)*float32(hpRatio), barHeight-2, color.RGBA{R: uint8(255 * (1 - hpRatio)), G: uint8(220 * hpRatio), B: 140, A: 220}, false)
	}
}

func (g *game) drawProjectiles(screen *ebiten.Image) {
	for _, p := range g.projectiles {
		if !p.alive {
			continue
		}
		vector.DrawFilledCircle(screen, float32(p.pos.X), float32(p.pos.Y), 8, p.color, false)
	}
}

func (g *game) drawUI(screen *ebiten.Image) {
	info := fmt.Sprintf("Gold: %d    Lives: %d    Wave: %d/%d    Score: %d", g.gold, g.lives, min(g.waveIndex+1, len(g.waves)), len(g.waves), g.score)
	drawText(screen, info, 24, 34, color.RGBA{240, 230, 255, 255}, 1.2)
	drawText(screen, "Left click: place tower (80 gold)  |  Right click: upgrade (120 gold)", 24, screenHeight-40, color.RGBA{210, 210, 230, 255}, 1.0)
	if g.waveIndex >= len(g.waves) && len(g.enemies) == 0 {
		drawCenteredText(screen, "All waves cleared!", screenWidth/2, 60, color.RGBA{180, 255, 200, 255}, 1.4)
	}
}

func (g *game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func main() {
	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("Celestial Bloom - Go Tower Defense")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	g := newGame()
	if err := ebiten.RunGame(g); err != nil {
		panic(err)
	}
}

func createBackground() *ebiten.Image {
	img := ebiten.NewImage(screenWidth, screenHeight)
	centerX := float64(screenWidth) / 2
	centerY := float64(screenHeight) / 2
	maxDist := math.Hypot(centerX, centerY)

	for y := 0; y < screenHeight; y++ {
		for x := 0; x < screenWidth; x++ {
			dx := float64(x) - centerX
			dy := float64(y) - centerY
			dist := math.Hypot(dx, dy)
			t := dist / maxDist
			r := uint8(30 + 120*(1-t))
			g := uint8(20 + 70*t)
			b := uint8(80 + 150*(1-t))
			img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}

	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 8; i++ {
		cx := rnd.Float64() * screenWidth
		cy := rnd.Float64() * screenHeight
		radius := 160 + rnd.Float64()*240
		tint := vibrantColor(i)
		for y := 0; y < screenHeight; y++ {
			for x := 0; x < screenWidth; x++ {
				dx := float64(x) - cx
				dy := float64(y) - cy
				dist := math.Hypot(dx, dy)
				if dist > radius {
					continue
				}
				fade := 1 - dist/radius
				r0, g0, b0, a0 := img.At(x, y).RGBA()
				blend := fade * 0.35
				nr := clamp(float64(r0>>8)+(float64(tint.R)-float64(r0>>8))*blend, 0, 255)
				ng := clamp(float64(g0>>8)+(float64(tint.G)-float64(g0>>8))*blend, 0, 255)
				nb := clamp(float64(b0>>8)+(float64(tint.B)-float64(b0>>8))*blend, 0, 255)
				img.Set(x, y, color.RGBA{uint8(nr), uint8(ng), uint8(nb), uint8(a0 >> 8)})
			}
		}
	}

	return img
}

func drawText(dst *ebiten.Image, value string, x, y int, clr color.Color, scale float64) {
	face := getFace(scale)
	text.Draw(dst, value, face, x, y, clr)
}

func drawCenteredText(dst *ebiten.Image, value string, x, y int, clr color.Color, scale float64) {
	face := getFace(scale)
	bounds, _ := font.BoundString(face, value)
	width := (bounds.Max.X - bounds.Min.X).Ceil()
	text.Draw(dst, value, face, x-width/2, y, clr)
}

func vibrantColor(i int) color.RGBA {
	palette := []color.RGBA{
		{R: 120, G: 220, B: 255, A: 255},
		{R: 254, G: 174, B: 120, A: 255},
		{R: 186, G: 120, B: 255, A: 255},
		{R: 140, G: 255, B: 170, A: 255},
		{R: 255, G: 120, B: 200, A: 255},
		{R: 255, G: 236, B: 120, A: 255},
	}
	return palette[i%len(palette)]
}

func clamp(value, minVal, maxVal float64) float64 {
	if value < minVal {
		return minVal
	}
	if value > maxVal {
		return maxVal
	}
	return value
}

func distance(a, b vec2) float64 {
	return math.Hypot(a.X-b.X, a.Y-b.Y)
}

func distancePointToSegment(p, a, b vec2) float64 {
	ap := vec2{p.X - a.X, p.Y - a.Y}
	ab := vec2{b.X - a.X, b.Y - a.Y}
	abLen2 := ab.X*ab.X + ab.Y*ab.Y
	if abLen2 == 0 {
		return distance(p, a)
	}
	t := (ap.X*ab.X + ap.Y*ab.Y) / abLen2
	t = math.Max(0, math.Min(1, t))
	closest := vec2{a.X + ab.X*t, a.Y + ab.Y*t}
	return distance(p, closest)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var (
	baseTT    *opentype.Font
	baseFace  font.Face
	faceCache = map[float64]font.Face{}
)

func init() {
	var err error
	baseTT, err = opentype.Parse(fonts.MPlus1pRegular_ttf)
	if err != nil {
		panic(err)
	}
	baseFace, err = opentype.NewFace(baseTT, &opentype.FaceOptions{Size: 22, DPI: 72})
	if err != nil {
		panic(err)
	}
}

func getFace(scale float64) font.Face {
	if face, ok := faceCache[scale]; ok {
		return face
	}
	size := 22 * scale
	if size < 10 {
		size = 10
	}
	face, err := opentype.NewFace(baseTT, &opentype.FaceOptions{Size: size, DPI: 72})
	if err != nil {
		return baseFace
	}
	faceCache[scale] = face
	return face
}
