package rtk

// registry maps filter names to their Filter struct. Populated at package init
// — mirrors 9router registry.js REGISTRY map. AutoDetect reads from this map;
// callers should not mutate it.
var registry = map[string]Filter{
	NameGitDiff:       {Name: NameGitDiff, Apply: gitDiffFilter},
	NameGitStatus:     {Name: NameGitStatus, Apply: gitStatusFilter},
	NameGrep:          {Name: NameGrep, Apply: grepFilter},
	NameFind:          {Name: NameFind, Apply: findFilter},
	NameTree:          {Name: NameTree, Apply: treeFilter},
	NameLs:            {Name: NameLs, Apply: lsFilter},
	NameSearchList:    {Name: NameSearchList, Apply: searchListFilter},
	NameReadNumbered:  {Name: NameReadNumbered, Apply: readNumberedFilter},
	NameDedupLog:      {Name: NameDedupLog, Apply: dedupLogFilter},
	NameSmartTruncate: {Name: NameSmartTruncate, Apply: smartTruncateFilter},
}

// aliases mirrors 9router registry.js ALIASES (rg→grep, fd→find).
var aliases = map[string]string{
	"rg": NameGrep,
	"fd": NameFind,
}

// LookupFilter returns the Filter for a name (or alias). Returns zero-value
// Filter (nil Apply) when unknown — callers must check.
func LookupFilter(name string) Filter {
	if alias, ok := aliases[name]; ok {
		name = alias
	}
	return registry[name]
}
