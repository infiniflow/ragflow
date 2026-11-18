//go:build !ee

package admin

func validateSystemSettingEE(string, string) (string, error) {
	return "", nil
}
