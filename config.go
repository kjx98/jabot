package jabot

type Config struct {
	Tuling Tuling `yaml:"tuling"`
	Jid    string
	Passwd string
}

type Tuling struct {
	URL    string `yaml:"url"`
	KeyAPI string `yaml:"APIkey"`
}

func NewConfig(key string) Config {
	url := tulingURL

	if key == "" {
		key = "808811ad0fd34abaa6fe800b44a9556a"
	}
	var cfg = Config{Tuling{URL: url,
		KeyAPI: key}, "", "",
	}
	return cfg
}
