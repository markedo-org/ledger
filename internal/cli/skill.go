package cli

import "fmt"

// SkillInstall is the command a human runs. ledger does not exec it.
const SkillInstall = "npx skills add markedo-org/ledger -s task-ledger"

func Skill(_ []string) int {
	fmt.Println(SkillInstall)
	return 0
}
