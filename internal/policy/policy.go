package policy

type Policy struct {
	Name string
}

func Default() Policy {
	return Policy{Name: "default"}
}
