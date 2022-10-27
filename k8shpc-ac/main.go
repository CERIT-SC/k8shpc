package main

// Things needed for webhook
// 1. Webhook Configuration (YAML)
// 2. Contacting webhook (URL/SERVICE)
// 3. Webhook itself
// 4. Webhook Server

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	admissionv1 "k8s.io/api/admission/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
)

var (
	infoLogger                                     *log.Logger
	errorLogger                                    *log.Logger
	deserializer                                   runtime.Decoder
	tag, image                                     string
	envVarsEmpty, labelsEmpty                      bool
	k8smemScaled, k8scpuScaled                     int
	extMem, extCPU, extGPU, k8sMem, k8sCPU, k8sGPU *int
)

func init() {
	infoLogger = log.New(os.Stderr, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)
	errorLogger = log.New(os.Stderr, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)
	deserializer = serializer.NewCodecFactory(runtime.NewScheme()).UniversalDeserializer()
}

func main() {
	extMem = flag.Int("ext_mem", 0, "Maximum node memory size in the external system in bytes.")
	extCPU = flag.Int("ext_cpu", 0, "Maximum node CPU size in the external system.")
	extGPU = flag.Int("ext_gpu", 0, "Maximum node GPU amount in the external system.")
	k8sMem = flag.Int("k8s_mem", 0, "Maximum node memory size in Kubernetes in bytes.")
	k8sCPU = flag.Int("k8s_cpu", 0, "Maximum node CPU size in Kubernetes.")
	k8sGPU = flag.Int("k8s_gpu", 0, "Maximum node GPU amount in Kubernetes.")
	flag.Parse()

	if flag.Lookup("ext_mem").Value.String() == "0" || flag.Lookup("ext_cpu").Value.String() == "0" ||
		flag.Lookup("k8s_mem").Value.String() == "0" || flag.Lookup("k8s_cpu").Value.String() == "0" {
		errorLogger.Println("Some flags not set, exiting.")
		panic(errors.New("Some command line arguments not set."))
	}

	k8smemScaled = int(math.Floor((float64(*k8sMem) / 100) * 70))
	k8scpuScaled = int(math.Floor((float64(*k8sCPU) / 100) * 70))

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
		sendHeaderErrorResponse(w, fmt.Sprintf("unmarshalling body into admission review object not successful: %v", err))
		return
	} else if admissionReviewReq.Request == nil {
		sendHeaderErrorResponse(w, fmt.Sprintf("using admission review not possible: request field is nil: %v", err))
		return
	}

	// check job with label received although mwh conf should have done that
	if admissionReviewReq.Request.Resource.Resource != "jobs" && admissionReviewReq.Request.Resource.Version != "v1" &&
		admissionReviewReq.Request.Resource.Group != "batch" {
		sendHeaderErrorResponse(w, fmt.Sprintf("wrong resource type: %v", err))
		return
	}

	job := batchv1.Job{}
	err = json.Unmarshal(admissionReviewReq.Request.Object.Raw, &job)
	if err != nil {
		sendHeaderErrorResponse(w, fmt.Sprintf("deserializing job not successful: %v", err))
		return
	}

	if val, ok := job.Labels["hpctransfer"]; !ok || val != "yes" {
		sendHeaderErrorResponse(w, fmt.Sprintf("job is not valid for mutation: label 'hpctransfer' not found "+
			"or not set to 'yes'"))
		return
	}

	if len(job.Spec.Template.Spec.Containers) != 1 {
		sendResponse(admissionReviewReq, w, nil, true, "sending back response, spec not changed (multiple containers)")
		return
	}

	if len(job.Spec.Template.Spec.Containers[0].Ports) != 0 {
		sendResponse(admissionReviewReq, w, nil, true, "sending back response, spec not changed (exposed ports)")
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

	//replaceCommand := []string{"/bin/bash", "-c", "/srv/start.sh"}
	replaceCommand := []string{"/bin/bash", "-c", "sleep infinity"}

	patches = replacePodSpecItem("containers/0/image", fmt.Sprintf("%s:%s", image, tag), patches)
	patches = replacePodSpecItem("containers/0/command", replaceCommand, patches)
	patches = replacePodSpecItem("automountServiceAccountToken", true, patches)

	envVarsEmpty = len(job.Spec.Template.Spec.Containers[0].Env) == 0

	for i, cmd := range job.Spec.Template.Spec.Containers[0].Command {
		patches = addEnvVar("CMD_"+strconv.Itoa(i), cmd, patches)
	}
	patches = addEnvVar("CONTAINER", job.Spec.Template.Spec.Containers[0].Image, patches)

	patches = addEnvVarFromField("NAMESPACE", "metadata.namespace", false, patches)
	patches = addEnvVarFromField("POD_NAME", "metadata.name", false, patches)

	patches = addEnvVarFromField("CPUR", "requests.cpu", true, patches)
	patches = addEnvVarFromField("CPUL", "limits.cpu", true, patches)
	patches = addEnvVarFromField("MEMR", "requests.memory", true, patches)
	patches = addEnvVarFromField("MEML", "limits.memory", true, patches)

	labelsEmpty = len(job.Spec.Template.Labels) == 0
	patches = addLabelToPod("app", job.Name, patches)

	for i, arg := range job.Spec.Template.Spec.Containers[0].Args {
		patches = addEnvVar("ARG_"+strconv.Itoa(i), arg, patches)
	}

	vmp := getPVCMountPath(job)
	for pvcname, mp := range vmp {
		patches = addEnvVar(pvcname, mp, patches)
	}

	patchesM, err = json.Marshal(patches)
	if err != nil {
		sendHeaderErrorResponse(w, fmt.Sprintf("marshalling patches not successful: %v", err))
		return
	}

	admissionReviewResponse := generateResponse(admissionReviewReq, patchesM, true, "")

	jout, err := json.Marshal(admissionReviewResponse)
	if err != nil {
		sendHeaderErrorResponse(w, fmt.Sprintf("marshalling response not successful: %v", err))
		return
	}
	infoLogger.Println("sending back response, job spec changed")
	w.WriteHeader(200)
	w.Write(jout)
}

func serveHealth(writer http.ResponseWriter, request *http.Request) {
	msg := fmt.Sprintf("healthy uri %s", request.RequestURI)
	infoLogger.Println(msg)
	writer.WriteHeader(200)
	writer.Write([]byte(msg))
}

func sendHeaderErrorResponse(w http.ResponseWriter, msg string) {
	errorLogger.Println(msg)
	w.WriteHeader(400)
	w.Write([]byte(msg))
}

func sendResponse(admissionReviewReq admissionv1.AdmissionReview, w http.ResponseWriter, patchesM []byte, allow bool, msg string) {
	admissionReviewResponse := generateResponse(admissionReviewReq, patchesM, allow, msg)
	jout, err := json.Marshal(admissionReviewResponse)
	if err != nil {
		sendHeaderErrorResponse(w, fmt.Sprintf("marshalling response not successful: %v", err))
		return
	}
	infoLogger.Println(msg)
	w.WriteHeader(200)
	w.Write(jout)
	return
}

func generateResponse(admissionReviewReq admissionv1.AdmissionReview, patchesM []byte, allow bool, msg string) admissionv1.AdmissionReview {
	reviewResponse := &admissionv1.AdmissionResponse{
		UID:     admissionReviewReq.Request.UID,
		Allowed: allow,
	}

	if !allow {
		reviewResponse.Result.Status = msg
		reviewResponse.Result.Code = 403
	}
	if allow && patchesM != nil {
		patchType := admissionv1.PatchTypeJSONPatch
		reviewResponse.Patch = patchesM
		reviewResponse.PatchType = &patchType
	}

	admissionReviewResponse := admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "admission.k8s.io/v1",
			Kind:       "AdmissionReview",
		},
		Response: reviewResponse,
	}

	return admissionReviewResponse
}

func checkResourcesAvailability() {

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

func getPVCMountPath(job batchv1.Job) map[string]string {
	volumes := job.Spec.Template.Spec.Volumes
	volumeMounts := job.Spec.Template.Spec.Containers[0].VolumeMounts
	pvcnames := map[string]string{}
	for _, v := range volumes {
		if v.VolumeSource.PersistentVolumeClaim != nil {
			pvcnames[v.Name] = v.VolumeSource.PersistentVolumeClaim.ClaimName
		}
	}

	volumeMountpath := map[string]string{}
	for _, vm := range volumeMounts {
		if pvcnames[vm.Name] != "" {
			name := fmt.Sprintf("PVC_%s", strings.ReplaceAll(pvcnames[vm.Name], "-", "_"))
			volumeMountpath[name] = vm.MountPath
		}
	}
	return volumeMountpath
}
