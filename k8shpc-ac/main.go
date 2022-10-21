package main

// Things needed for webhook
// 1. Webhook Configuration (YAML)
// 2. Contacting webhook (URL/SERVICE)
// 3. Webhook itself
// 4. Webhook Server

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	admissionv1 "k8s.io/api/admission/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"log"
	"net/http"
	"os"
)

var (
	infoLogger                *log.Logger
	warningLogger             *log.Logger
	errorLogger               *log.Logger
	deserializer              runtime.Decoder
	tag, image                string
	envVarsEmpty, labelsEmpty bool
)

func init() {
	// init loggers
	infoLogger = log.New(os.Stderr, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)
	warningLogger = log.New(os.Stderr, "WARNING: ", log.Ldate|log.Ltime|log.Lshortfile)
	errorLogger = log.New(os.Stderr, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)
	deserializer = serializer.NewCodecFactory(runtime.NewScheme()).UniversalDeserializer()
}

func main() {
	http.HandleFunc("/mutate", serveMutateJobs)
	http.HandleFunc("/health", serveHealth)

	certPath := "/etc/tls/ca.crt"
	keyPath := "/etc/tls/tls.key"
	infoLogger.Print("Listening on port 8443...")

	_, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		errorLogger.Println("TLS certs not found, exiting.")
		panic(err)
	}

	err = http.ListenAndServeTLS(":8443", certPath, keyPath, nil)
	if err != nil {
		errorLogger.Println("Can't serve TLS server, exiting.")
		panic(err)
	}
}

func serveHealth(writer http.ResponseWriter, request *http.Request) {
	msg := fmt.Sprintf("healthy uri %s", request.RequestURI)
	infoLogger.Println(msg)
	writer.WriteHeader(200)
	writer.Write([]byte(msg))
}

func replacePodSpecItem(path string, value interface{}, patches []map[string]interface{}) []map[string]interface{} {
	patch := map[string]interface{}{
		"op":    "replace",
		"path":  "/spec/template/spec/" + path,
		"value": value,
	}
	patches = append(patches, patch)
	return patches
}

func addLabelToPod(labelName, value string, patches []map[string]interface{}) []map[string]interface{} {
	patch := map[string]interface{}{
		"op":   "add",
		"path": "/spec/template/metadata/labels",
	}
	if labelsEmpty {
		patch["value"] = map[string]string{labelName: value}
		labelsEmpty = false
	} else {
		patch["path"] = fmt.Sprintf("%s/%s", patch["path"], labelName)
		patch["value"] = value
	}
	patches = append(patches, patch)
	return patches
}

// Currently only to the first container
func addEnvVar(envname string, value string, patches []map[string]interface{}) []map[string]interface{} {
	patch := map[string]interface{}{
		"op":   "add",
		"path": "/spec/template/spec/containers/0/env",
	}
	envVar := v1.EnvVar{Name: envname, Value: value}
	if envVarsEmpty {
		patch["value"] = []v1.EnvVar{envVar}
		envVarsEmpty = false
	} else {
		patch["path"] = fmt.Sprintf("%v/-", patch["path"])
		patch["value"] = envVar
	}
	patches = append(patches, patch)
	return patches
}

// Currently only to the first container
func addEnvVarFromField(envname, field string, isResource bool, patches []map[string]interface{}) []map[string]interface{} {
	patch := map[string]interface{}{
		"op":   "add",
		"path": "/spec/template/spec/containers/0/env",
	}

	envVar := v1.EnvVar{Name: envname}
	if !isResource {
		envVar.ValueFrom = &v1.EnvVarSource{
			FieldRef: &v1.ObjectFieldSelector{FieldPath: field},
		}
	} else {
		envVar.ValueFrom = &v1.EnvVarSource{
			ResourceFieldRef: &v1.ResourceFieldSelector{
				Resource: field,
			},
		}
	}

	if envVarsEmpty {
		patch["value"] = []v1.EnvVar{envVar}
		envVarsEmpty = false
	} else {
		patch["path"] = fmt.Sprintf("%v/-", patch["path"])
		patch["value"] = envVar
	}
	patches = append(patches, patch)
	return patches
}

func serveMutateJobs(w http.ResponseWriter, r *http.Request) {
	infoLogger.Println("received mutation request")

	if r.Header.Get("Content-Type") != "application/json" {
		errorLogger.Println("admission request must be application/json content-type")
		panic("wrong content-type")
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		errorLogger.Println("error while reading body %s", err.Error())
		panic(err)
	}

	var admissionReviewReq admissionv1.AdmissionReview

	_, _, err = deserializer.Decode(body, nil, &admissionReviewReq)
	if err != nil {
		msg := fmt.Sprintf("unmarshalling body into admission review object not successful: %v", err)
		errorLogger.Println(msg)
		w.WriteHeader(400)
		w.Write([]byte(msg))
		return
	} else if admissionReviewReq.Request == nil {
		msg := fmt.Sprintf("using admission review not possible: request field is nil: %v", err)
		errorLogger.Println(msg)
		w.WriteHeader(400)
		w.Write([]byte(msg))
		return
	}

	// check job with label received although mwh conf should have done that
	if admissionReviewReq.Request.Resource.Resource != "jobs" && admissionReviewReq.Request.Resource.Version != "v1" &&
		admissionReviewReq.Request.Resource.Group != "batch" {
		msg := fmt.Sprintf("wrong resource type: %v", err)
		errorLogger.Println(msg)
		w.WriteHeader(400)
		w.Write([]byte(msg))
		return
	}

	job := batchv1.Job{}
	err = json.Unmarshal(admissionReviewReq.Request.Object.Raw, &job)
	if err != nil {
		msg := fmt.Sprintf("deserializing job not successful: %v", err)
		errorLogger.Println(msg)
		w.WriteHeader(400)
		w.Write([]byte(msg))
		return
	}

	if val, ok := job.Labels["hpctransfer"]; !ok || val != "yes" {
		msg := fmt.Sprintf("job is not valid for mutation: label 'hpctransfer' not found or not set to 'yes'")
		errorLogger.Println(msg)
		w.WriteHeader(400)
		w.Write([]byte(msg))
		return
	}

	var patchesM []byte
	var patches []map[string]interface{}

	tag = os.Getenv("TAG")
	if len(tag) == 0 {
		tag = "latest"
	}
	image = os.Getenv("IMAGE")
	if len(image) == 0 {
		tag = ""
	}

	replaceCommand := []string{"/bin/bash", "-c", "/srv/start.sh"}

	patches = replacePodSpecItem("containers/0/image", fmt.Sprintf("%s:%s", image, tag), patches)
	patches = replacePodSpecItem("containers/0/command", replaceCommand, patches)
	patches = replacePodSpecItem("automountServiceAccountToken", true, patches)

	envVarsEmpty = len(job.Spec.Template.Spec.Containers[0].Env) == 0
	command := job.Spec.Template.Spec.Containers[0].Command[len(job.Spec.Template.Spec.Containers[0].Command)-1]

	patches = addEnvVar("COMMAND", command, patches)
	patches = addEnvVar("CONTAINER", job.Spec.Template.Spec.Containers[0].Image, patches)

	patches = addEnvVarFromField("NAMESPACE", "metadata.namespace", false, patches)
	patches = addEnvVarFromField("POD_NAME", "metadata.name", false, patches)

	patches = addEnvVarFromField("CPUR", "requests.cpu", true, patches)
	patches = addEnvVarFromField("CPUL", "limits.cpu", true, patches)
	patches = addEnvVarFromField("MEMR", "requests.memory", true, patches)
	patches = addEnvVarFromField("MEML", "limits.memory", true, patches)

	labelsEmpty = len(job.Spec.Template.Labels) == 0
	patches = addLabelToPod("app", job.Name, patches)

	// ARGS FOR JOB
	//var args string
	//for _, s := range job.Spec.Template.Spec.Containers[0].Args {
	//	args = s + " "
	//}
	//aEnv := map[string]string{"name": "ARGS", "value": args}
	//aPatch := map[string]interface{}{
	//	"op":    "add",
	//	"path":  "/spec/template/spec/containers/0/env/-1",
	//	"value": aEnv,
	//}
	//patches = append(patches, aPatch)

	//// VOLUME MOUNTS
	var mountPaths string
	for i, vm := range job.Spec.Template.Spec.Containers[0].VolumeMounts {
		if i == len(job.Spec.Template.Spec.Containers[0].VolumeMounts)-1 {
			mountPaths += vm.MountPath
		} else {
			mountPaths += fmt.Sprintf("%s,", vm.MountPath)
		}
	}
	patches = addEnvVar("MNT", mountPaths, patches)

	patchesM, err = json.Marshal(patches)
	if err != nil {
		msg := fmt.Sprintf("marshalling patches not successful: %v", err)
		errorLogger.Println(msg)
		w.WriteHeader(400)
		w.Write([]byte(msg))
		return
	}

	patchType := admissionv1.PatchTypeJSONPatch
	reviewResponse := &admissionv1.AdmissionResponse{
		UID:       admissionReviewReq.Request.UID,
		Allowed:   true,
		Patch:     patchesM,
		PatchType: &patchType,
	}

	admissionReviewResponse := admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "admission.k8s.io/v1",
			Kind:       "AdmissionReview",
		},
		Response: reviewResponse,
	}

	jout, err := json.Marshal(admissionReviewResponse)
	if err != nil {
		msg := fmt.Sprintf("marshalling response not successful: %v", err)
		errorLogger.Println(msg)
		w.WriteHeader(400)
		w.Write([]byte(msg))
		return
	}
	infoLogger.Println("sending back response")
	w.WriteHeader(200)
	w.Write(jout)
}
