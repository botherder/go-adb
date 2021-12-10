module github.com/danielpaulus/go-adb

go 1.16

require (
	github.com/docopt/docopt-go v0.0.0-20180111231733-ee0de3bc6815
	github.com/google/gousb v2.1.0+incompatible
	github.com/gorilla/mux v1.8.0
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.7.0
	github.com/stretchr/testify v1.6.1
)

replace github.com/google/gousb => github.com/danielpaulus/gousb v1.1.5
