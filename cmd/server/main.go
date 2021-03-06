package main

import (
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"encoding/json"

	"log"

	"github.com/satori/go.uuid"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	mobile "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"

	"k8s.io/client-go/pkg/api/v1"
)

var k8client *kubernetes.Clientset
var pushClient *upsClient

const NamespaceKey = "NAMESPACE"
const ActionAdded = "ADDED"
const ActionDeleted = "DELETED"
const SecretTypeKey = "secretType"
const BindingSecretType = "mobile-client-binding-secret"
const BindingAppType = "appType"

const BindingClientId = "clientId"
const BindingGoogleKey = "googleKey"
const BindingProjectNumber = "projectNumber"

const UpsSecretName = "unified-push-server"
const GoogleKey = "googleKey"

// This is required because importing core/v1/Secret leads to a double import and redefinition
// of log_dir
type BindingSecret struct {
	metav1.TypeMeta              `json:",inline"`
	metav1.ObjectMeta            `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Data       map[string][]byte `json:"data,omitempty" protobuf:"bytes,2,rep,name=data"`
	StringData map[string]string `json:"stringData,omitempty" protobuf:"bytes,4,rep,name=stringData"`
}

// Deletes the binding secret after the sync operation has completed
func deleteSecret(name string) {
	err := k8client.CoreV1().Secrets(os.Getenv(NamespaceKey)).Delete(name, nil)
	if err != nil {
		log.Fatal("Error creating config map", err)
	} else {
		log.Printf("Secret `%s` has been deleted", name)
	}
}

func createAndroidVariantConfigMap(variant *androidVariant) {
	variantName := variant.Name + "-config-map"

	payload := v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: variantName,
			Labels: map[string]string{
				"mobile":       "enabled",
				"serviceName":  "ups",
				"resourceType": "binding",
			},
		},
		Data: map[string]string{
			"name":          variant.Name,
			"description":   variant.Description,
			"variantID":     variant.VariantID,
			"secret":        variant.Secret,
			"googleKey":     variant.GoogleKey,
			"projectNumber": variant.ProjectNumber,
			"type":          "android",
		},
	}

	_, err := k8client.CoreV1().ConfigMaps(os.Getenv(NamespaceKey)).Create(&payload)
	if err != nil {
		log.Fatal("Error creating config map", err)
	} else {
		log.Printf("Config map `%s` for variant created", variantName)
	}
}

func handleAndroidVariant(key string, name string, pn string) {
	// Only instantiate the push client here because we need to wait for the ups secret to
	// be available
	if pushClient == nil {
		pushClient = pushClientOrDie()
	}

	if pushClient.hasAndroidVariant(key) == nil {
		payload := &androidVariant{
			ProjectNumber: pn,
			GoogleKey:     key,
			variant: variant{
				Name:      name,
				VariantID: uuid.NewV4().String(),
				Secret:    uuid.NewV4().String(),
			},
		}

		log.Print("Creating a new android variant", payload)
		success, variant := pushClient.createAndroidVariant(payload)
		if success {
			createAndroidVariantConfigMap(variant)
		} else {
			log.Fatal("No variant has been created in UPS, skipping config map")
		}
	} else {
		log.Printf("A variant for google key '%s' already exists", key)
	}
}

func handleDeleteAndroidVariant(secret *BindingSecret) {
	if _, ok := secret.Data[GoogleKey]; !ok {
		log.Println("Secret does not contain a google key, can't delete android variant")
		return
	}

	googleKey := string(secret.Data[GoogleKey])
	log.Printf("Deleting config map associated with google key `%s`", googleKey)

	// Get all config maps
	configs, err := k8client.CoreV1().ConfigMaps(os.Getenv(NamespaceKey)).List(metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}

	configMapDeleted := false

	// Filter config maps to identify the one associated with the given google key
	for _, config := range configs.Items {
		if config.Labels["resourceType"] == "binding" && config.Data[GoogleKey] == googleKey {
			name := config.Name
			log.Printf("Config map with name `%s` has a matching google key", name)

			// Delete the config map
			err := k8client.CoreV1().ConfigMaps(os.Getenv(NamespaceKey)).Delete(name, nil)
			if err != nil {
				log.Fatal("Error deleting config map with name `%s`", name, err)
				break;
			}

			log.Printf("Config map `%s` has been deleted", name)
			configMapDeleted = true
			break
		}
	}

	if pushClient == nil {
		pushClient = pushClientOrDie()
	}

	// Delete the UPS variant only if the associated config map has been deleted
	if configMapDeleted == true {
		pushClient.deleteVariant(googleKey)
	}
}

func handleAddSecret(obj runtime.Object) {
	raw, _ := json.Marshal(obj)
	var secret = BindingSecret{}
	json.Unmarshal(raw, &secret)

	if val, ok := secret.Labels[SecretTypeKey]; ok && val == BindingSecretType {
		appType := string(secret.Data[BindingAppType])

		if appType == "Android" {
			log.Print("A mobile binding secret of type `Android` was added")
			clientId := string(secret.Data[BindingClientId])
			googleKey := string(secret.Data[BindingGoogleKey])
			projectNumber := string(secret.Data[BindingProjectNumber])
			handleAndroidVariant(googleKey, clientId, projectNumber)
		}

		// Always delete the secret after handling it regardless of any new resources
		// was created
		deleteSecret(secret.Name)
	}
}

func handleDeleteSecret(obj runtime.Object) {
	raw, _ := json.Marshal(obj)
	var secret = BindingSecret{}
	json.Unmarshal(raw, &secret)

	for _, ref := range secret.ObjectMeta.OwnerReferences {
		if ref.Kind == "ServiceBinding" {
			handleDeleteAndroidVariant(&secret)
			break;
		}
	}
}

func watchLoop() {
	events, err := k8client.CoreV1().Secrets(os.Getenv(NamespaceKey)).Watch(metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}

	for update := range events.ResultChan() {
		switch action := update.Type; action {
		case ActionAdded:
			handleAddSecret(update.Object)
		case ActionDeleted:
			handleDeleteSecret(update.Object)
		default:
			log.Print("Unhandled action:", action)
		}
	}
}

func convertSecretToUpsSecret(s *mobile.Secret) *pushApplication {
	return &pushApplication{
		ApplicationId: string(s.Data["applicationId"]),
	}
}

func kubeOrDie(config *rest.Config) *kubernetes.Clientset {
	k8client, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	return k8client
}

func pushClientOrDie() *upsClient {
	upsSecret, err := k8client.CoreV1().Secrets(os.Getenv(NamespaceKey)).Get(UpsSecretName, metav1.GetOptions{})
	if err != nil {
		panic(err.Error())
	}

	return &upsClient{
		config: convertSecretToUpsSecret(upsSecret),
	}
}

func main() {
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}

	k8client = kubeOrDie(config)

	log.Print("Entering watch loop")

	for {
		watchLoop()
	}
}
