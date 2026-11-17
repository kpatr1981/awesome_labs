package exercise

// TrackOrder is the order the tracks are listed in; tracks not in it come
// after, as they appear in the catalogue.
var TrackOrder = []string{"docker", "kubernetes", "offsec", "linux", "networking", "cloud", "data", "defensive",
	"programming", "datascience", "git", "terraform", "ansible", "algorithms"}

var trackNames = map[string]string{
	"docker": "Docker", "kubernetes": "Kubernetes", "offsec": "Offensive Security", "linux": "Linux",
	"networking": "Networking", "cloud": "Cloud", "data": "Data", "defensive": "Defensive Security",
	"programming": "Programming", "datascience": "Data Science", "git": "Git", "terraform": "Terraform",
	"ansible": "Ansible", "algorithms": "Algorithms",
}

// TrackName is a track's full display name (the id itself for an unknown track).
func TrackName(t string) string {
	if n, ok := trackNames[t]; ok {
		return n
	}
	return t
}
