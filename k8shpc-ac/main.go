package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	admissionv1 "k8s.io/api/admission/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	infoLogger                                     *log.Logger
	errorLogger                                    *log.Logger
	deserializer                                   runtime.Decoder
	tag, image                                     string
	envVarsEmpty, labelsEmpty                      bool
	k8smemScaled, k8scpuScaled                     int64
	extMem, extCPU, extGPU, k8sMem, k8sCPU, k8sGPU *int64
	nodeMaxMem, nodeMaxCPU, nodeMaxGPU             chan int64
)

func init() {
	infoLogger = log.New(os.Stderr, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)
	errorLogger = log.New(os.Stderr, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)
	deserializer = serializer.NewCodecFactory(runtime.NewScheme()).UniversalDeserializer()
}

func main() {
	extMem = flag.Int64("ext_mem", 0, "Maximum node memory size in the external system in bytes.")
	extCPU = flag.Int64("ext_cpu", 0, "Maximum node CPU size in the external system.")
	extGPU = flag.Int64("ext_gpu", 0, "Maximum node GPU amount in the external system.")
	k8sMem = flag.Int64("k8s_mem", 0, "Maximum node memory size in Kubernetes in bytes.")
	k8sCPU = flag.Int64("k8s_cpu", 0, "Maximum node CPU size in Kubernetes.")
	k8sGPU = flag.Int64("k8s_gpu", 0, "Maximum node GPU amount in Kubernetes.")
	flag.Parse()

	if flag.Lookup("ext_mem").Value.String() == "0" || flag.Lookup("ext_cpu").Value.String() == "0" ||
		flag.Lookup("k8s_mem").Value.String() == "0" || flag.Lookup("k8s_cpu").Value.String() == "0" {
		errorLogger.Println("Some flags not set, exiting.")
		panic(errors.New("Some command line arguments not set."))
	}

	k8smemScaled = int64(math.Floor((float64(*k8sMem) / 100) * 70))
	k8scpuScaled = int64(math.Floor((float64(*k8sCPU) / 100) * 70))

	nodeMaxMem = make(chan int64)
	nodeMaxCPU = make(chan int64)
	nodeMaxGPU = make(chan int64)
	go queryCurrentK8sResources(nodeMaxMem, nodeMaxCPU, nodeMaxGPU)
	infoLogger.Println("Starting queryCurrentK8sResources go routine, sleeping 10s.")
	time.Sleep(10 * time.Second)

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

	val, ok := job.Labels["hpctransfer"]
	if !ok || (val != "can" && val != "must" && val != "cooperative") {
		sendHeaderErrorResponse(w, fmt.Sprintf("job is not valid for mutation: label 'hpctransfer' not found "+
			"or not set to one of ['can','must']"))
		return
	}

	if len(job.Spec.Template.Spec.Containers) != 1 {
		sendResponse(admissionReviewReq, w, nil, true,
			"sending back response, spec not changed (multiple containers)")
		return
	}

	if len(job.Spec.Template.Spec.Containers[0].Ports) != 0 {
		sendResponse(admissionReviewReq, w, nil, true,
			"sending back response, spec not changed (exposed ports)")
		return
	}

	// can, must, cooperative
	max := checkMaxResources(
		job.Spec.Template.Spec.Containers[0].Resources.Requests.Name("nvidia.com/gpu", "0").Value(),
		job.Spec.Template.Spec.Containers[0].Resources.Requests.Memory().Value(),
		job.Spec.Template.Spec.Containers[0].Resources.Requests.Cpu().Value())

	if max == -1 {
		sendResponse(admissionReviewReq, w, nil, false,
			"sending back response, resources can not be accommodated anywhere")
		return
	}
	if val == "can" { // can checks cluster maximum
		if max == 0 {
			sendResponse(admissionReviewReq, w, nil, true,
				"sending back response, spec not changed (possible to accommodate in K8s)")
			return
		}
	} else if val == "cooperative" { // cooperative checks currently available
		current := checkCurrentResources(job.Spec.Template.Spec.Containers[0].Resources.Requests.Name("nvidia.com/gpu", "0").Value(),
			job.Spec.Template.Spec.Containers[0].Resources.Requests.Memory().Value(),
			job.Spec.Template.Spec.Containers[0].Resources.Requests.Cpu().Value())

		if !current {
			sendResponse(admissionReviewReq, w, nil, true,
				"sending back response, spec not changed (possible to accommodate in K8s)")
			return
		}
	}

	var patchesM []byte
	var patches []map[string]interface{}

	tag = os.Getenv("TAG")
	if len(tag) == 0 {
		tag = "latest"
	}
	image = os.Getenv("IMAGE")
	if len(image) == 0 {
		sendResponse(admissionReviewReq, w, nil, false,
			"sending back response, proxy image not set")
		return
	}

	envVarsEmpty = len(job.Spec.Template.Spec.Containers[0].Env) == 0
	for i, arg := range job.Spec.Template.Spec.Containers[0].Args {
		patches = addEnvVar(fmt.Sprintf("ARG_%02d", i), arg, patches)
	}
	for _, envVar := range job.Spec.Template.Spec.Containers[0].Env {
		patches = addEnvVar(fmt.Sprintf("ENV_%s", envVar.Name), envVar.Value, patches)
	}
	vmp := getPVCMountPath(job)
	for pvcname, mp := range vmp {
		patches = addEnvVar(pvcname, mp, patches)
	}
	for i, cmd := range job.Spec.Template.Spec.Containers[0].Command {
		patches = addEnvVar("CMD_"+strconv.Itoa(i), cmd, patches)
	}
	patches = addEnvVar("CONTAINER", job.Spec.Template.Spec.Containers[0].Image, patches)

	patches = addEnvVarFromField("NAMESPACE", "metadata.namespace", false, patches)
	patches = addEnvVarFromField("POD_NAME", "metadata.name", false, patches)

	gpuR := job.Spec.Template.Spec.Containers[0].Resources.Requests.Name("nvidia.com/gpu", resource.DecimalSI)
	cpuR := job.Spec.Template.Spec.Containers[0].Resources.Requests.Cpu()
	cpuL := job.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu()
	memR := job.Spec.Template.Spec.Containers[0].Resources.Requests.Memory().Value()
	memL := job.Spec.Template.Spec.Containers[0].Resources.Limits.Memory().Value()
	patches = addEnvVar("GPUR", gpuR.String(), patches)
	patches = addEnvVar("CPUR", cpuR.String(), patches)
	patches = addEnvVar("CPUL", cpuL.String(), patches)
	patches = addEnvVar("MEMR", strconv.FormatInt(memR, 10), patches)
	patches = addEnvVar("MEML", strconv.FormatInt(memL, 10), patches)

	labelsEmpty = len(job.Spec.Template.Labels) == 0
	patches = addLabelToPod("app", job.Name, patches)

	replaceCommand := []string{"/srv/start.sh"}

	patches = replacePodSpecItem("containers/0/image", fmt.Sprintf("%s:%s", image, tag), patches)
	patches = replacePodSpecItem("containers/0/command", replaceCommand, patches)
	patches = replacePodSpecItem("automountServiceAccountToken", true, patches)

	patches = replacePodSpecItem("containers/0/resources/limits/cpu", "100m", patches)
	patches = replacePodSpecItem("containers/0/resources/requests/cpu", "100m", patches)
	patches = replacePodSpecItem("containers/0/resources/limits/memory", "512Mi", patches)
	patches = replacePodSpecItem("containers/0/resources/requests/memory", "512Mi", patches)

	if gpuR.String() != "0" {
		patches = removePodSpecItem("containers/0/resources/limits/nvidia.com~1gpu", patches)
		patches = removePodSpecItem("containers/0/resources/requests/nvidia.com~1gpu", patches)
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

// -1 = fail | 0 = k8s | 1 = ext
func checkMaxResources(gpur, memr, cpur int64) int {
	if gpur > 0 {
		if gpur > *k8sGPU { // || gpur > kga
			if gpur > *extGPU || memr > *extMem || cpur > *extCPU {
				errorLogger.Println("failing, accommodating not possible in any system")
				return -1
			} else {
				infoLogger.Println("moving to external system")
				return 1
			}
		} else {
			if memr > k8smemScaled || cpur > k8scpuScaled {
				if gpur > *extGPU || memr > *extMem || cpur > *extCPU {
					errorLogger.Println("failing, accommodating not possible in any system")
					return -1
				}
				infoLogger.Println("moving to external system")
				return 1
			}
			infoLogger.Println("staying in K8s")
			return 0
		}
	}
	if memr > k8smemScaled {
		if memr > *extMem || cpur > *extCPU {
			errorLogger.Println("failing, accommodating not possible in any system")
			return -1
		}
		infoLogger.Println("moving to external system")
		return 1
	}
	if cpur > k8scpuScaled {
		if memr > *extMem || cpur > *extCPU {
			errorLogger.Println("failing, accommodating not possible in any system")
			return -1
		}
		infoLogger.Println("moving to external system")
		return 1
	}
	infoLogger.Println("staying in K8s")
	return 0
}

// true = ext | false = K8s
func checkCurrentResources(gpur, memr, cpur int64) bool {
	maxFreeGPU := <-nodeMaxGPU
	maxFreeCPU := <-nodeMaxCPU
	maxFreeMem := <-nodeMaxMem
	infoLogger.Println(fmt.Sprintf("Current max GPU %d, max CPU %d, max MEM %d.",
		maxFreeGPU, maxFreeCPU, maxFreeMem))

	if gpur > 0 && gpur > maxFreeGPU { // GPU request cannot be accommodated in K8s right now
		infoLogger.Println(fmt.Sprintf(
			"GPU request(%d) larger than available(%d); moving to external system", gpur, maxFreeGPU))
		return true
	}
	if memr > maxFreeMem {
		rM := memr / 1024 / 1024
		maxG := maxFreeMem / 1024 / 1024 / 1024
		infoLogger.Println(fmt.Sprintf(
			"MEM request(%d MB, %d GB) larger than available MEM (%d GB); moving to external system",
			rM, rM/1024, maxG))
		return true
	}
	if cpur > maxFreeCPU {
		infoLogger.Println(fmt.Sprintf(
			"CPU request(%d) larger than available CPU (%d); moving to external system", cpur, maxFreeCPU))
		return true
	}
	infoLogger.Println("staying in K8s")
	return false
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

func removePodSpecItem(path string, patches []map[string]interface{}) []map[string]interface{} {
	patch := map[string]interface{}{
		"op":   "remove",
		"path": "/spec/template/spec/" + path,
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
			i := 0
			pvcname := strings.ReplaceAll(pvcnames[vm.Name], "-", "_")
			name := fmt.Sprintf("PVC_%02d_%s", i, pvcname)
			for volumeMountpath[name] != "" {
				i += 1
				name = fmt.Sprintf("PVC_%02d_%s", i, pvcname)
			}
			volumeMountpath[name] = vm.MountPath
		}
	}
	return volumeMountpath
}

func queryCurrentK8sResources(nodeMaxMem, nodeMaxCPU, nodeMaxGPU chan int64) {
	defer func() { fmt.Println("Routine is gone") }()
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	cs, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	var nodemaxcpu, nodemaxmem, nodemaxgpu, reqcpu, reqmem, reqgpu int64
	for {
		nodes, err := cs.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			panic(err.Error())
		}
		nodemaxcpu = 0
		nodemaxmem = 0
		nodemaxgpu = 0
		for _, node := range nodes.Items {
			currPods, err := cs.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
				FieldSelector: "spec.nodeName=" + node.Name,
			})
			if err != nil {
				panic(err.Error())
			}
			reqcpu = 0
			reqmem = 0
			reqgpu = 0
			for _, pod := range currPods.Items {
				for _, container := range pod.Spec.Containers {
					reqcpu += container.Resources.Requests.Cpu().Value()
					reqmem += container.Resources.Requests.Memory().Value()
					reqgpu += container.Resources.Requests.Name("nvidia.com/gpu", resource.DecimalSI).Value()
				}
			}
			freecpu := node.Status.Allocatable.Cpu().Value() - reqcpu
			freemem := node.Status.Allocatable.Memory().Value() - reqmem
			freegpu := node.Status.Allocatable.Name("nvidia.com/gpu", resource.DecimalSI).Value() - reqgpu
			if freecpu > nodemaxcpu {
				nodemaxcpu = freecpu
			}
			if freemem > nodemaxmem {
				nodemaxmem = freemem
			}
			if freegpu > nodemaxgpu {
				nodemaxgpu = freegpu
			}
		}
		nodeMaxGPU <- nodemaxgpu
		nodeMaxCPU <- nodemaxcpu
		nodeMaxMem <- nodemaxmem
	}
}
