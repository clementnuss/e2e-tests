package e2etests

import (
	"os"
	"testing"

	_ "k8s.io/client-go/plugin/pkg/client/auth/azure"
	"sigs.k8s.io/e2e-framework/klient/conf"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
)

var testenv env.Environment

func TestMain(m *testing.M) {
	testenv = env.New()
	path := conf.ResolveKubeConfigFile()
	cfg := envconf.NewWithKubeConfig(path)
	testenv = env.NewWithConfig(cfg)
	namespace := envconf.RandomName("sample-ns", 16)
	testenv.Setup(
		envfuncs.CreateNamespace(namespace),
	)
	testenv.Finish(
		envfuncs.DeleteNamespace(namespace),
	)
	os.Exit(testenv.Run(m))
}
