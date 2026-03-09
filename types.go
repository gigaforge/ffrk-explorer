package main

// ---------- data types ----------

type Character struct {
	Realm      string
	Name       string
	Img        string
	ID         string
	ATK        int
	MAG        int
	Schools    map[string]int // school name -> max rarity (1-6)
	DamageType string         // "PHY", "MAG", "MND", or combinations like "PHY/MAG"
}

type StatusEffect struct {
	Name        string
	Description string
	Duration    string
}

type BurstCommand struct {
	Img            string
	Name           string
	Type           string
	Target         string
	Element        string
	Time           string
	Effects        string
	School         string
	ID             string
	MatchedEffects []StatusEffect
}

type SynchroAbility struct {
	Img            string
	Name           string
	Slot           string
	Condition      string
	Type           string
	Target         string
	Element        string
	Time           string
	Effects        string
	School         string
	ID             string
	ConditionID    string
	MatchedEffects []StatusEffect
}

type ZenithAbility struct {
	Img            string
	Name           string
	Type           string
	Target         string
	Element        string
	Time           string
	Effects        string
	School         string
	ID             string
	MatchedEffects []StatusEffect
}

type BraveLevel struct {
	Level          string
	Type           string
	Target         string
	Element        string
	Time           string
	Effects        string
	MatchedEffects []StatusEffect
}

type BraveCommand struct {
	Name      string
	School    string
	Condition string
	Levels    []BraveLevel // 0-3
}

type TriggerAbility struct {
	SourceType     string
	Img            string
	Name           string
	Type           string
	Target         string
	Formula        string
	Multiplier     string
	Element        string
	Time           string
	Effects        string
	School         string
	ID             string
	MatchedEffects []StatusEffect
}

type SoulBreak struct {
	Character        string
	Img              string
	Name             string
	Tier             string
	SBVer            string
	Type             string
	Multiplier       string
	Element          string
	Time             string
	Effects          string
	ID               string
	MatchedEffects   []StatusEffect
	BurstCommands    []BurstCommand
	SynchroAbilities []SynchroAbility
	ZenithAbilities  []ZenithAbility
	TriggerAbilities []TriggerAbility
	DualShift        *SoulBreak
	ArcaneDyad       *SoulBreak
	BraveCommand     *BraveCommand
}

type HeroAbility struct {
	Character string
	Img       string
	Name      string
	HAVer     string
	Type      string
	Element   string
	Time      string
	Effects   string
	School    string
	SB        string
	ID        string
}

type LegendMateria struct {
	Img            string
	Name           string
	Tier           string
	LMVer          string
	Effect         string
	Lensable       bool
	ID             string
	MatchedEffects []StatusEffect
}

type SearchResult struct {
	Character      Character
	HeroAbilities  []HeroAbility
	LegendMateria  []LegendMateria
	SoulBreaks     []SoulBreak
}

type RealmGroup struct {
	Realm      string
	Characters []Character
}

type CharDetail struct {
	Character       Character
	SoulBreaks      []SoulBreak
	HeroAbilities   []HeroAbility
	LegendMateria   []LegendMateria
	LoggedIn        bool
	OwnedSoulbreaks map[string]bool
}

type AppData struct {
	csvDir           string
	Characters       []Character
	SoulBreaks       map[string][]SoulBreak
	HeroAbilities    map[string][]HeroAbility
	BurstCommands    map[string][]BurstCommand
	SynchroAbilities map[string][]SynchroAbility
	ZenithAbilities  map[string][]ZenithAbility
	BraveCommands    map[string]*BraveCommand
	TriggerAbilities map[string][]TriggerAbility // sbName → []TriggerAbility
	LegendMateria    map[string][]LegendMateria  // character → []LegendMateria
	StatusEffects    map[string]StatusEffect
	statusMatchTerms []string
	RealmGroups      []RealmGroup
	CharByID         map[string]*Character
	TierNames        []string
}

// ---------- user types & globals ----------

type User struct {
	ID           int    `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
	APIKey       string `json:"api_key"`
}

// ---------- CSV auto-update ----------

type csvSheet struct {
	Filename        string
	GID             string
	ExpectedHeaders []string
}

type PartyMember struct {
	Character     Character
	HeroAbilities []HeroAbility
	LegendMateria []LegendMateria
	SoulBreaks    []SoulBreak
}

type PartyData struct {
	Members         []PartyMember
	AllCharacters   []Character
	LoggedIn        bool
	OwnedSoulbreaks map[string]bool
}

type SearchData struct {
	Results         []SearchResult
	Truncated       bool
	MaxResults      int
	LoggedIn        bool
	OwnedSoulbreaks map[string]bool
	Query           struct {
		Character  string
		Realm      string
		DamageType string
		Tier       string
		Element    string
		Imperil    string
		Schools    string
		SchoolMode string
	}
}
