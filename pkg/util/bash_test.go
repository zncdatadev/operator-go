package util_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/util"
)

var _ = Describe("RemoveVectorShutdownFileCommand", func() {
	Context("Testing RemoveVectorShutdownFileCommand function", func() {
		It("should generate correct command string ", func() {
			command := util.RemoveVectorShutdownFileCommand()
			Expect(command).Should(Equal("rm -f /kubedoop/log/_vector/shutdown"))
		})
	})
})

var _ = Describe("CreateVectorShutdownFileCommand", func() {
	It("should generate correct command string ", func() {
		command := util.CreateVectorShutdownFileCommand()
		Expect(command).Should(Equal("mkdir -p /kubedoop/log/_vector && touch /kubedoop/log/_vector/shutdown"))
	})
})

var _ = Describe("ExportPodAddress", func() {
	Context("correctly generates the command", func() {
		It("supports valid logDir input", func() {
			command := util.ExportPodAddress()
			Expect(command).Should(Equal(`if [[ -d /kubedoop/listener ]]; then
  export POD_ADDRESS=$(cat /kubedoop/listener/default-address/address)
  for i in /kubedoop/listener/default-address/ports/*; do
      export $(basename $i | tr a-z A-Z)_PORT="$(cat $i)"
  done
fi`))
		})
	})
})
