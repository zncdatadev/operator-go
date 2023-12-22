package v1alphav1 //nolint:typecheck
import (
	"github.com/zncdata-labs/operator-go/pkg/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatabaseSpec defines the desired connection info of Database
type DatabaseSpec struct {
	//+kubebuilder:validation:Required
	Name string `json:"name,omitempty"`
	//+kubebuilder:validation:Required
	Reference string `json:"reference,omitempty"`
	//+kubebuilder:validation:Required
	Credential *DatabaseCredentialSpec `json:"credential,omitempty"`
}

// DatabaseCredentialSpec include  Username and Password or ExistSecret. ExistSecret include Username and Password ,it is encrypted by base64.
type DatabaseCredentialSpec struct {
	ExistSecret string `json:"existingSecret,omitempty"`
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Database is the Schema for the databases API
type Database struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseSpec         `json:"spec,omitempty"`
	Status status.ZncdataStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DatabaseList contains a list of Database
type DatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Database `json:"items"`
}
