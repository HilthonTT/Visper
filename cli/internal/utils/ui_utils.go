package utils

func FooterWidth(fullWidth int) int {
	return fullWidth/3 - 2
}

func FullFooterHeight(footerHeight int, toggleFooter bool) int {
	if toggleFooter {
		return footerHeight + 2
	}
	return 0
}
