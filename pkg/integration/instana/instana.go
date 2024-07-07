package instana

import (
	"github.com/k8sgpt-ai/k8sgpt/pkg/common"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"github.com/spf13/viper"
)

type Instana struct {
	client client.Client
}

func NewInstana() *Instana {
	cfg, err := config.GetConfig()
	if err != nil {
		panic(err)
	}

	k8sClient, err := client.New(cfg, client.Options{})
	if err != nil {
		panic(err)
	}

	return &Instana{
		client: k8sClient,
	}
}

func (i *Instana) GetAnalyzerName() []string {
	return []string{"SelfHostedInstallCheck"}
}

func (i *Instana) GetNamespace() (string, error) {
	return "", nil
}

func (i *Instana) OwnsAnalyzer(analyzer string) bool {
	for _, az := range i.GetAnalyzerName() {
		if analyzer == az {
			return true
		}
	}
	return false
}

func (i *Instana) Deploy(namespace string) error {
	return nil
}

func (i *Instana) UnDeploy(namespace string) error {
	return nil
}

func (i *Instana) isFilterActive() bool {
	activeFilters := viper.GetStringSlice("active_filters")

	for _, filter := range i.GetAnalyzerName() {
		for _, af := range activeFilters {
			if af == filter {
				return true
			}
		}
	}
	return false
}

func (i *Instana) IsActivate() bool {
	if i.isFilterActive() {
		return true
	} else {
		return false
	}
}

func (i *Instana) AddAnalyzer(mergedMap *map[string]common.IAnalyzer) {
	(*mergedMap)["SelfHostedInstallCheck"] = &SelHostedInstallAnalyzer{
		instana: i,
	}
}
