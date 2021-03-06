package ruler

import (
	"crypto/md5"
	"sort"

	"github.com/cortexproject/cortex/pkg/util"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/prometheus/pkg/rulefmt"
	"github.com/spf13/afero"
	"gopkg.in/yaml.v2"
)

// mapper is designed to enusre the provided rule sets are identical
// to the on-disk rules tracked by the prometheus manager
type mapper struct {
	Path string // Path specifies the directory in which rule files will be mapped.

	FS afero.Fs
}

func newMapper(path string) *mapper {
	return &mapper{
		Path: path,
		FS:   afero.NewOsFs(),
	}
}

func (m *mapper) MapRules(user string, ruleConfigs map[string][]rulefmt.RuleGroup) (bool, []string, error) {
	anyUpdated := false
	filenames := []string{}

	// user rule files will be stored as `/<path>/<userid>/filename`
	path := m.Path + "/" + user + "/"
	err := m.FS.MkdirAll(path, 0777)
	if err != nil {
		return false, nil, err
	}

	// write all rule configs to disk
	for filename, groups := range ruleConfigs {
		fullFileName := path + filename

		fileUpdated, err := m.writeRuleGroupsIfNewer(groups, fullFileName)
		if err != nil {
			return false, nil, err
		}
		filenames = append(filenames, fullFileName)
		anyUpdated = anyUpdated || fileUpdated
	}

	// and clean any up that shouldn't exist
	existingFiles, err := afero.ReadDir(m.FS, path)
	if err != nil {
		return false, nil, err
	}

	for _, existingFile := range existingFiles {
		fullFileName := path + existingFile.Name()
		ruleGroups := ruleConfigs[existingFile.Name()]

		if ruleGroups == nil {
			err = m.FS.Remove(fullFileName)
			if err != nil {
				level.Warn(util.Logger).Log("msg", "unable to remove rule file on disk", "file", fullFileName, "err", err)
			}
			anyUpdated = true
		}
	}

	return anyUpdated, filenames, nil
}

func (m *mapper) writeRuleGroupsIfNewer(groups []rulefmt.RuleGroup, filename string) (bool, error) {
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Name > groups[j].Name
	})

	rgs := rulefmt.RuleGroups{Groups: groups}

	d, err := yaml.Marshal(&rgs)
	if err != nil {
		return false, err
	}

	_, err = m.FS.Stat(filename)
	if err == nil {
		current, err := afero.ReadFile(m.FS, filename)
		if err != nil {
			return false, err
		}
		newHash := md5.New()
		currentHash := md5.New()

		// bailout if there is no update
		if string(currentHash.Sum(current)) == string(newHash.Sum(d)) {
			return false, nil
		}
	}

	level.Info(util.Logger).Log("msg", "updating rule file", "file", filename)
	err = afero.WriteFile(m.FS, filename, d, 0777)
	if err != nil {
		return false, err
	}

	return true, nil
}
