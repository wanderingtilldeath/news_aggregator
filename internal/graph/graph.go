// Package graph builds the knowledge graph of the news from the entities each story names: which
// people, organisations, places and laws are in the news most, the stories about each, and who is
// named alongside whom. It reads rows the enrichment pass already stored, so it needs no model.
package graph

import (
	"sort"
	"strings"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

// StoryRef is a story about an entity.
type StoryRef struct {
	ID     string    `json:"id"`
	Title  string    `json:"title"`
	Source string    `json:"source"`
	Why    string    `json:"why,omitempty"`
	Time   time.Time `json:"time"`
}

// Edge is another entity named in the same stories, and in how many.
type Edge struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Node is an entity: how many stories name it, the newest of them, and who is named with it.
type Node struct {
	Name    string     `json:"name"`
	Count   int        `json:"count"`
	Stories []StoryRef `json:"stories"`
	Related []Edge     `json:"related"`
}

// Link joins two of the entities in the graph, with the number of stories that name both.
type Link struct {
	A     string `json:"a"`
	B     string `json:"b"`
	Count int    `json:"count"`
}

// Graph is the top entities, with the links between them.
type Graph struct {
	Entities []Node `json:"entities"`
	Links    []Link `json:"links"`
}

// maxLinks keeps the picture readable: only the strongest links between the top entities are drawn.
const maxLinks = 120

// Build returns the topN entities by number of stories (then by name), each with its newest
// perEntity stories (stories must come newest first) and its relatedN most frequent companions. The
// same name in a different case is one entity, shown the way it was first written.
func Build(stories []store.EntityStory, topN, perEntity, relatedN int) Graph {
	display := map[string]string{}
	count := map[string]int{}
	refs := map[string][]StoryRef{}
	pairs := map[[2]string]int{}

	for _, st := range stories {
		keys := make([]string, 0, len(st.Entities))
		seen := map[string]bool{}
		for _, name := range st.Entities {
			key := strings.ToLower(strings.TrimSpace(name))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			if _, ok := display[key]; !ok {
				display[key] = strings.TrimSpace(name)
			}
			keys = append(keys, key)
			count[key]++
			if len(refs[key]) < perEntity {
				refs[key] = append(refs[key], StoryRef{ID: st.ID, Title: st.Title, Source: st.Source, Why: st.Why, Time: st.Time})
			}
		}
		for i, a := range keys {
			for _, b := range keys[i+1:] {
				pairs[orderedPair(a, b)]++
			}
		}
	}

	order := make([]string, 0, len(count))
	for key := range count {
		order = append(order, key)
	}
	sort.Slice(order, func(i, j int) bool {
		if count[order[i]] != count[order[j]] {
			return count[order[i]] > count[order[j]]
		}
		return order[i] < order[j]
	})
	if topN > 0 && len(order) > topN {
		order = order[:topN]
	}

	// Companions of each entity, from every pair it is in.
	companions := map[string][]Edge{}
	for pair, n := range pairs {
		companions[pair[0]] = append(companions[pair[0]], Edge{Name: display[pair[1]], Count: n})
		companions[pair[1]] = append(companions[pair[1]], Edge{Name: display[pair[0]], Count: n})
	}

	g := Graph{Entities: make([]Node, 0, len(order)), Links: []Link{}}
	inGraph := map[string]bool{}
	for _, key := range order {
		inGraph[key] = true
		rel := companions[key]
		sort.Slice(rel, func(i, j int) bool {
			if rel[i].Count != rel[j].Count {
				return rel[i].Count > rel[j].Count
			}
			return rel[i].Name < rel[j].Name
		})
		if relatedN > 0 && len(rel) > relatedN {
			rel = rel[:relatedN]
		}
		if rel == nil {
			rel = []Edge{}
		}
		g.Entities = append(g.Entities, Node{Name: display[key], Count: count[key], Stories: refs[key], Related: rel})
	}

	for pair, n := range pairs {
		if inGraph[pair[0]] && inGraph[pair[1]] {
			g.Links = append(g.Links, Link{A: display[pair[0]], B: display[pair[1]], Count: n})
		}
	}
	sort.Slice(g.Links, func(i, j int) bool {
		if g.Links[i].Count != g.Links[j].Count {
			return g.Links[i].Count > g.Links[j].Count
		}
		if g.Links[i].A != g.Links[j].A {
			return g.Links[i].A < g.Links[j].A
		}
		return g.Links[i].B < g.Links[j].B
	})
	if len(g.Links) > maxLinks {
		g.Links = g.Links[:maxLinks]
	}

	return g
}

func orderedPair(a, b string) [2]string {
	if a < b {
		return [2]string{a, b}
	}

	return [2]string{b, a}
}
