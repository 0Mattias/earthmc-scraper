package api

import "encoding/json"

// ============================================================
// Server
// ============================================================

type ServerResponse struct {
	Version    string          `json:"version"`
	MoonPhase  string          `json:"moonPhase"`
	Timestamps ServerTimestamp  `json:"timestamps"`
	Status     ServerStatus    `json:"status"`
	Stats      ServerStats     `json:"stats"`
	VoteParty  ServerVoteParty `json:"voteParty"`
}

type ServerTimestamp struct {
	NewDayTime      int64 `json:"newDayTime"`
	ServerTimeOfDay int64 `json:"serverTimeOfDay"`
}

type ServerStatus struct {
	HasStorm     bool `json:"hasStorm"`
	IsThundering bool `json:"isThundering"`
}

type ServerStats struct {
	Time              int64 `json:"time"`
	FullTime          int64 `json:"fullTime"`
	MaxPlayers        int   `json:"maxPlayers"`
	NumOnlinePlayers  int   `json:"numOnlinePlayers"`
	NumOnlineNomads   int   `json:"numOnlineNomads"`
	NumResidents      int   `json:"numResidents"`
	NumNomads         int   `json:"numNomads"`
	NumTowns          int   `json:"numTowns"`
	NumTownBlocks     int   `json:"numTownBlocks"`
	NumNations        int   `json:"numNations"`
	NumQuarters       int   `json:"numQuarters"`
	NumCuboids        int   `json:"numCuboids"`
}

type ServerVoteParty struct {
	Target       int `json:"target"`
	NumRemaining int `json:"numRemaining"`
}

// ============================================================
// Online Players
// ============================================================

type OnlineResponse struct {
	Count   int           `json:"count"`
	Players []ListEntry   `json:"players"`
}

// ============================================================
// Shared list entry (name + uuid)
// ============================================================

type ListEntry struct {
	Name string `json:"name"`
	UUID string `json:"uuid"`
}

// ============================================================
// Map players.json
// ============================================================

type MapPlayersResponse struct {
	Max     int         `json:"max"`
	Players []MapPlayer `json:"players"`
}

type MapPlayer struct {
	World       string `json:"world"`
	Name        string `json:"name"`
	X           int    `json:"x"`
	Y           int    `json:"y"`
	Z           int    `json:"z"`
	DisplayName string `json:"display_name"`
	UUID        string `json:"uuid"`
	Yaw         int    `json:"yaw"`
}

// ============================================================
// Player Detail (POST /players response)
// ============================================================

type PlayerDetail struct {
	Name          string           `json:"name"`
	UUID          string           `json:"uuid"`
	Title         *string          `json:"title"`
	Surname       *string          `json:"surname"`
	FormattedName *string          `json:"formattedName"`
	About         *string          `json:"about"`
	Town          *ListEntry       `json:"town"`
	Nation        *ListEntry       `json:"nation"`
	Timestamps    *PlayerTimestamp  `json:"timestamps"`
	Status        *PlayerStatus    `json:"status"`
	Stats         *PlayerStats     `json:"stats"`
	Perms         *Perms           `json:"perms"`
	Ranks         *PlayerRanks     `json:"ranks"`
	Friends       []ListEntry      `json:"friends"`
}

type PlayerTimestamp struct {
	Registered   *int64 `json:"registered"`
	JoinedTownAt *int64 `json:"joinedTownAt"`
	LastOnline   *int64 `json:"lastOnline"`
}

type PlayerStatus struct {
	IsOnline  bool `json:"isOnline"`
	IsNPC     bool `json:"isNPC"`
	IsMayor   bool `json:"isMayor"`
	IsKing    bool `json:"isKing"`
	HasTown   bool `json:"hasTown"`
	HasNation bool `json:"hasNation"`
}

type PlayerStats struct {
	Balance    float64 `json:"balance"`
	NumFriends int     `json:"numFriends"`
}

type PlayerRanks struct {
	TownRanks   []string `json:"townRanks"`
	NationRanks []string `json:"nationRanks"`
}

// ============================================================
// Town Detail (POST /towns response)
// ============================================================

type TownDetail struct {
	Name        string             `json:"name"`
	UUID        string             `json:"uuid"`
	Board       *string            `json:"board"`
	Founder     *string            `json:"founder"`
	Wiki        *string            `json:"wiki"`
	Mayor       *ListEntry         `json:"mayor"`
	Nation      *ListEntry         `json:"nation"`
	Timestamps  *TownTimestamp     `json:"timestamps"`
	Status      *TownStatus        `json:"status"`
	Stats       *TownStats         `json:"stats"`
	Perms       *Perms             `json:"perms"`
	Coordinates *TownCoordinates   `json:"coordinates"`
	Residents   []ListEntry        `json:"residents"`
	Trusted     []ListEntry        `json:"trusted"`
	Outlaws     []ListEntry        `json:"outlaws"`
	Quarters    []string           `json:"quarters"`
	Ranks       map[string][]string `json:"ranks"`
}

type TownTimestamp struct {
	Registered    *int64 `json:"registered"`
	JoinedNationAt *int64 `json:"joinedNationAt"`
	RuinedAt      *int64 `json:"ruinedAt"`
}

type TownStatus struct {
	IsPublic           bool `json:"isPublic"`
	IsOpen             bool `json:"isOpen"`
	IsNeutral          bool `json:"isNeutral"`
	IsCapital          bool `json:"isCapital"`
	IsOverClaimed      bool `json:"isOverClaimed"`
	IsRuined           bool `json:"isRuined"`
	IsForSale          bool `json:"isForSale"`
	HasNation          bool `json:"hasNation"`
	HasOverclaimShield bool `json:"hasOverclaimShield"`
	CanOutsidersSpawn  bool `json:"canOutsidersSpawn"`
}

type TownStats struct {
	NumTownBlocks int      `json:"numTownBlocks"`
	MaxTownBlocks int      `json:"maxTownBlocks"`
	NumResidents  int      `json:"numResidents"`
	NumTrusted    int      `json:"numTrusted"`
	NumOutlaws    int      `json:"numOutlaws"`
	Balance       float64  `json:"balance"`
	ForSalePrice  *float64 `json:"forSalePrice"`
}

type TownCoordinates struct {
	Spawn      *SpawnCoord `json:"spawn"`
	HomeBlock  []int       `json:"homeBlock"`
	TownBlocks [][]int     `json:"townBlocks"`
}

type SpawnCoord struct {
	World string  `json:"world"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Z     float64 `json:"z"`
	Pitch float32 `json:"pitch"`
	Yaw   float32 `json:"yaw"`
}

// ============================================================
// Nation Detail (POST /nations response)
// ============================================================

type NationDetail struct {
	Name          string              `json:"name"`
	UUID          string              `json:"uuid"`
	Board         *string             `json:"board"`
	DynmapColour  *string             `json:"dynmapColour"`
	DynmapOutline *string             `json:"dynmapOutline"`
	Wiki          *string             `json:"wiki"`
	King          *ListEntry          `json:"king"`
	Capital       *ListEntry          `json:"capital"`
	Timestamps    *NationTimestamp     `json:"timestamps"`
	Status        *NationStatus       `json:"status"`
	Stats         *NationStats        `json:"stats"`
	Coordinates   *NationCoordinates  `json:"coordinates"`
	Residents     []ListEntry         `json:"residents"`
	Towns         []ListEntry         `json:"towns"`
	Allies        []ListEntry         `json:"allies"`
	Enemies       []ListEntry         `json:"enemies"`
	Sanctioned    []ListEntry         `json:"sanctioned"`
	Ranks         map[string][]string `json:"ranks"`
}

type NationTimestamp struct {
	Registered *int64 `json:"registered"`
}

type NationStatus struct {
	IsPublic  bool `json:"isPublic"`
	IsOpen    bool `json:"isOpen"`
	IsNeutral bool `json:"isNeutral"`
}

type NationStats struct {
	NumTownBlocks int     `json:"numTownBlocks"`
	NumResidents  int     `json:"numResidents"`
	NumTowns      int     `json:"numTowns"`
	NumAllies     int     `json:"numAllies"`
	NumEnemies    int     `json:"numEnemies"`
	Balance       float64 `json:"balance"`
}

type NationCoordinates struct {
	Spawn *SpawnCoord `json:"spawn"`
}

// ============================================================
// Shared permission structure
// ============================================================

type Perms struct {
	Build   []bool    `json:"build"`
	Destroy []bool    `json:"destroy"`
	Switch  []bool    `json:"switch"`
	ItemUse []bool    `json:"itemUse"`
	Flags   PermFlags `json:"flags"`
}

type PermFlags struct {
	PVP       bool `json:"pvp"`
	Explosion bool `json:"explosion"`
	Fire      bool `json:"fire"`
	Mobs      bool `json:"mobs"`
}

// ============================================================
// POST request bodies
// ============================================================

type PostQuery struct {
	Query []string `json:"query"`
}

// RawJSON is used for storing complete API responses as JSONB.
type RawJSON = json.RawMessage
