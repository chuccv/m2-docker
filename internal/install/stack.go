package install

import (
	"fmt"
	"regexp"
)

type Stack struct{ PHP, DB, Search, Node string }

var versionRe = regexp.MustCompile(`^2\.4\.(\d+)(-p\d+)?$`)

var stacks = map[string]Stack{
	"9": {"8.5", "11.4", "3.0.0", "22"},
	"8": {"8.4", "11.4", "2.19.1", "22"},
	"7": {"8.3", "10.6", "2.12.0", "22"},
	"6": {"8.2", "10.6", "2.5.0", "22"},
	"5": {"8.1", "10.4", "1.2.4", "22"},
	"4": {"8.1", "10.4", "1.2.4", "22"},
}

func StackFor(magentoVersion string) (Stack, error) {
	m := versionRe.FindStringSubmatch(magentoVersion)
	if m != nil {
		if s, ok := stacks[m[1]]; ok {
			return s, nil
		}
	}
	return Stack{}, fmt.Errorf("unsupported Magento version %q (supported: 2.4.4 - 2.4.9)", magentoVersion)
}
