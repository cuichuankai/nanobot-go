package mediaproviders

// Provider defines the interface for different media generation backends.
type Provider interface {
	GenerateImage(prompt, model string) (string, error)
	EditImage(prompt, imageURL, model string) (string, error)
	GenerateVideo(prompt, imageURL, model string) (string, error)
	GenerateAudio(input, model string) (string, error)
}
