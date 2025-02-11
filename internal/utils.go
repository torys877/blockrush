package internal

func handleErrors(errors *[]error, err error) {
	if err != nil {
		*errors = append(*errors, err)
	}
}
