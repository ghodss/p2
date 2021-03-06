package health

// SortOrder sorts the nodes in the list from least to most health.
type SortOrder struct {
	Nodes  []string
	Health map[string][]Result
}

func (s SortOrder) Len() int {
	return len(s.Nodes)
}

func (s SortOrder) Swap(i, j int) {
	tmp := s.Nodes[i]
	s.Nodes[i] = s.Nodes[j]
	s.Nodes[j] = tmp
}

func (s SortOrder) Less(i, j int) bool {
	iHealth := Unknown
	if res, ok := s.Health[s.Nodes[i]]; ok && len(res) > 0 {
		_, iHealth = FindWorst(res)
	}

	jHealth := Unknown
	if res, ok := s.Health[s.Nodes[j]]; ok && len(res) > 0 {
		_, jHealth = FindWorst(res)
	}

	return Compare(iHealth, jHealth) < 0
}
