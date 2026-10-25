package kb

import (
	"fmt"
	"strings"
	"testing"
)

func TestSmokeSearch(t *testing.T) {
	ix, err := Load("../../corpus")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("chunks:", ix.Len())
	q := "Какие обязательные этапы согласования проходит проект приказа?"
	qt := tokenize(q)
	fmt.Println("qkeys:", qt)
	res := ix.Search(q, 5)
	for _, s := range res {
		fmt.Printf("\n%s %.1f\n", s.Chunk.ID, s.Score)
		var hits []string
		for _, w := range qt {
			k := matchKey(w)
			n := s.Chunk.compiled.textKeys[k]
			tit := s.Chunk.compiled.titleSet[k]
			if n > 0 || tit {
				hits = append(hits, fmt.Sprintf("%s(t=%v,n=%d)", k, tit, n))
			}
		}
		fmt.Println("  hits:", strings.Join(hits, " "))
	}
	// raw head of p39 pre chunk
	c, _ := ix.Get("od184-p39|pre")
	if c != nil {
		fmt.Printf("\np39pre TEXT HEAD: %q\n", c.Text[:200])
	}
	c41, _ := ix.Get("od184-p41|pre")
	if c41 != nil {
		fmt.Printf("p41pre TEXT HEAD: %q\n", c41.Text[:300])
	} else {
		fmt.Println("no p41pre")
	}
}
