package generator

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
)

type Generator struct {
	adjectives []string
	nouns      []string
	verbs      []string
	colors     []string
	animals    []string
	objects    []string
	prefixes   []string
	suffixes   []string
}

func NewGenerator() *Generator {
	return &Generator{
		adjectives: []string{
			"Silent", "Mysterious", "Phantom", "Shadow", "Cosmic", "Electric",
			"Neon", "Crystal", "Velvet", "Quantum", "Digital", "Lunar",
			"Solar", "Arctic", "Desert", "Forest", "Ocean", "Storm",
			"Thunder", "Lightning", "Frost", "Ember", "Azure", "Crimson",
			"Golden", "Silver", "Mystic", "Ancient", "Modern", "Future",
			"Wild", "Gentle", "Fierce", "Swift", "Bold", "Bright",
			"Dark", "Pale", "Vivid", "Subtle", "Hidden", "Lost",
			"Found", "Broken", "Whole", "Eternal", "Fleeting", "Distant",
			"Near", "Remote", "Urban", "Rural", "Cosmic", "Stellar",
			"Nebula", "Void", "Echo", "Whisper", "Shout", "Murmur",
		},
		nouns: []string{
			"Wanderer", "Explorer", "Dreamer", "Thinker", "Seeker", "Hunter",
			"Guardian", "Watcher", "Keeper", "Traveler", "Nomad", "Pilgrim",
			"Sage", "Oracle", "Prophet", "Mystic", "Cipher", "Enigma",
			"Riddle", "Puzzle", "Mystery", "Secret", "Whisper", "Echo",
			"Shadow", "Ghost", "Phantom", "Spirit", "Spectre", "Wraith",
			"Dragon", "Phoenix", "Griffin", "Raven", "Wolf", "Bear",
			"Tiger", "Panther", "Falcon", "Hawk", "Eagle", "Owl",
			"Fox", "Lynx", "Jaguar", "Leopard", "Lion", "Cobra",
			"Viper", "Python", "Serpent", "Basilisk", "Hydra", "Kraken",
		},
		verbs: []string{
			"Running", "Flying", "Dancing", "Singing", "Coding", "Hacking",
			"Building", "Creating", "Destroying", "Wandering", "Exploring", "Seeking",
			"Finding", "Losing", "Winning", "Fighting", "Defending", "Attacking",
			"Hiding", "Revealing", "Whispering", "Shouting", "Laughing", "Crying",
			"Thinking", "Dreaming", "Waking", "Sleeping", "Rising", "Falling",
		},
		colors: []string{
			"Red", "Blue", "Green", "Yellow", "Purple", "Orange",
			"Pink", "Black", "White", "Gray", "Brown", "Cyan",
			"Magenta", "Crimson", "Scarlet", "Azure", "Cobalt", "Indigo",
			"Violet", "Amber", "Jade", "Ruby", "Emerald", "Sapphire",
			"Topaz", "Onyx", "Pearl", "Silver", "Gold", "Bronze",
		},
		animals: []string{
			"Wolf", "Fox", "Bear", "Tiger", "Lion", "Panther",
			"Eagle", "Hawk", "Falcon", "Raven", "Owl", "Crow",
			"Dragon", "Phoenix", "Griffin", "Sphinx", "Pegasus", "Unicorn",
			"Kraken", "Leviathan", "Behemoth", "Shark", "Whale", "Dolphin",
			"Octopus", "Jellyfish", "Mantis", "Spider", "Scorpion", "Viper",
		},
		objects: []string{
			"Blade", "Sword", "Arrow", "Shield", "Hammer", "Axe",
			"Spear", "Dagger", "Bow", "Staff", "Wand", "Orb",
			"Crown", "Throne", "Castle", "Tower", "Gate", "Bridge",
			"Mountain", "Valley", "River", "Ocean", "Star", "Moon",
			"Sun", "Comet", "Meteor", "Galaxy", "Nebula", "Void",
		},
		prefixes: []string{
			"Mr", "Ms", "Dr", "Sir", "Lord", "Lady",
			"Captain", "Major", "General", "Admiral", "Commander", "Chief",
			"Master", "Grand", "High", "Supreme", "Ultra", "Super",
			"Mega", "Hyper", "Neo", "Proto", "Meta", "Crypto",
		},
		suffixes: []string{
			"Jr", "Sr", "III", "IV", "V",
			"2000", "3000", "9000", "X", "XL", "Pro",
			"Max", "Ultra", "Prime", "Alpha", "Beta", "Omega",
			"Zero", "One", "Neo", "Cyber", "Tech", "Byte",
		},
	}
}

func (g *Generator) Generate() string {
	strategies := []func() string{
		g.AdjectiveNoun,
		g.ColorAnimal,
		g.VerbingNoun,
		g.AdjectiveAnimal,
		g.ColorObject,
		g.PrefixNoun,
		g.NounSuffix,
		g.AdjectiveNounNumber,
		g.ThreeWords,
		g.PhantomStyle,
		g.CyberStyle,
	}

	idx := g.secureRandom(len(strategies))
	return strategies[idx]()
}

// AdjectiveNoun generates usernames like "SilentWanderer"
func (g *Generator) AdjectiveNoun() string {
	adj := g.adjectives[g.secureRandom(len(g.adjectives))]
	noun := g.nouns[g.secureRandom(len(g.nouns))]
	return adj + noun
}

// ColorAnimal generates usernames like "CrimsonWolf"
func (g *Generator) ColorAnimal() string {
	color := g.colors[g.secureRandom(len(g.colors))]
	animal := g.animals[g.secureRandom(len(g.animals))]
	return color + animal
}

// VerbingNoun generates usernames like "FlyingPhoenix"
func (g *Generator) VerbingNoun() string {
	verb := g.verbs[g.secureRandom(len(g.verbs))]
	noun := g.nouns[g.secureRandom(len(g.nouns))]
	return verb + noun
}

// AdjectiveAnimal generates usernames like "SwiftFalcon"
func (g *Generator) AdjectiveAnimal() string {
	adj := g.adjectives[g.secureRandom(len(g.adjectives))]
	animal := g.animals[g.secureRandom(len(g.animals))]
	return adj + animal
}

// ColorObject generates usernames like "GoldenSword"
func (g *Generator) ColorObject() string {
	color := g.colors[g.secureRandom(len(g.colors))]
	object := g.objects[g.secureRandom(len(g.objects))]
	return color + object
}

// PrefixNoun generates usernames like "CaptainPhantom"
func (g *Generator) PrefixNoun() string {
	prefix := g.prefixes[g.secureRandom(len(g.prefixes))]
	noun := g.nouns[g.secureRandom(len(g.nouns))]
	return prefix + noun
}

// NounSuffix generates usernames like "PhantomPrime"
func (g *Generator) NounSuffix() string {
	noun := g.nouns[g.secureRandom(len(g.nouns))]
	suffix := g.suffixes[g.secureRandom(len(g.suffixes))]
	return noun + suffix
}

// AdjectiveNounNumber generates usernames like "DarkPhantom42"
func (g *Generator) AdjectiveNounNumber() string {
	adj := g.adjectives[g.secureRandom(len(g.adjectives))]
	noun := g.nouns[g.secureRandom(len(g.nouns))]
	num := g.secureRandom(100)
	return fmt.Sprintf("%s%s%d", adj, noun, num)
}

// ThreeWords generates usernames like "SilentCrimsonWolf"
func (g *Generator) ThreeWords() string {
	adj := g.adjectives[g.secureRandom(len(g.adjectives))]
	color := g.colors[g.secureRandom(len(g.colors))]
	animal := g.animals[g.secureRandom(len(g.animals))]
	return adj + color + animal
}

// PhantomStyle generates usernames like "phantom_shadow_42"
func (g *Generator) PhantomStyle() string {
	adj := g.adjectives[g.secureRandom(len(g.adjectives))]
	noun := g.nouns[g.secureRandom(len(g.nouns))]
	num := g.secureRandom(100)
	return fmt.Sprintf("%s_%s_%d", strings.ToLower(adj), strings.ToLower(noun), num)
}

// CyberStyle generates usernames like "cyber-wolf-x"
func (g *Generator) CyberStyle() string {
	adj := g.adjectives[g.secureRandom(len(g.adjectives))]
	animal := g.animals[g.secureRandom(len(g.animals))]
	suffix := g.suffixes[g.secureRandom(len(g.suffixes))]
	return fmt.Sprintf("%s-%s-%s", strings.ToLower(adj), strings.ToLower(animal), strings.ToLower(suffix))
}

func (g *Generator) GenerateWithStyle(style string) string {
	switch strings.ToLower(style) {
	case "adjective-noun":
		return g.AdjectiveNoun()
	case "color-animal":
		return g.ColorAnimal()
	case "verbing-noun":
		return g.VerbingNoun()
	case "adjective-animal":
		return g.AdjectiveAnimal()
	case "color-object":
		return g.ColorObject()
	case "prefix-noun":
		return g.PrefixNoun()
	case "noun-suffix":
		return g.NounSuffix()
	case "with-number":
		return g.AdjectiveNounNumber()
	case "three-words":
		return g.ThreeWords()
	case "phantom":
		return g.PhantomStyle()
	case "cyber":
		return g.CyberStyle()
	default:
		return g.Generate()
	}
}

func (g *Generator) GenerateBatch(count int) []string {
	usernames := make([]string, 0, count)
	seen := make(map[string]bool)

	for len(usernames) < count {
		username := g.Generate()
		if !seen[username] {
			seen[username] = true
			usernames = append(usernames, username)
		}
	}

	return usernames
}

func (g *Generator) secureRandom(max int) int {
	if max <= 0 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0
	}
	return int(n.Int64())
}
