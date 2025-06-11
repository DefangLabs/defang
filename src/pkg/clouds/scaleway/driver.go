package scaleway

import "github.com/scaleway/scaleway-sdk-go/scw"

type Scaleway struct {
	client *scw.Client
}

func New() (*Scaleway, error) {
	client, err := scw.NewClient()
	if err != nil {
		return nil, err
	}
	return &Scaleway{
		client: client,
	}, nil
}

func (s *Scaleway) Run() {

}

func (s *Scaleway) CreateUploadURL() (string, error) {
}
