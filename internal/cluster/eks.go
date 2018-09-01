package cluster

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/blang/semver"
	"github.com/kubernauts/tk8/internal/templates"
)

func EKSCreate() {
	kube, err := exec.LookPath("kubectl")
	if err != nil {
		log.Fatal("kubectl not found, kindly check")
	}
	fmt.Printf("Found kubectl at %s\n", kube)
	rr, err := exec.Command("kubectl", "version", "--client", "--short").Output()
	if err != nil {
		log.Fatal(err)
	}

	log.Println(string(rr))

	go parseTemplate(templates.Credentials, "./eks/credentials.tfvars", GetCredentials())
	go parseTemplate(templates.TerraformEKS, "./eks/terraform.tfvars", GetEKSConfig())

	//Check if kubectl version is greater or equal to 1.10

	parts := strings.Split(string(rr), " ")

	KubeCtlVer := strings.Replace((parts[2]), "v", "", -1)

	v1, err := semver.Make("1.10.0")
	v2, err := semver.Make(strings.TrimSpace(KubeCtlVer))

	if v2.LT(v1) {
		log.Fatalln("kubectl client version on this system is less than the required version 1.10.0")
	}

	// Check if a terraform state file aclready exists
	if _, err := os.Stat("./eks/terraform.tfstate"); err == nil {
		log.Fatalln("There is an existing cluster, please remove terraform.tfstate file or delete the installation before proceeding")
	}

	// Terraform Initialization and create the infrastructure

	log.Println("starting terraform init")

	terrInit := exec.Command("terraform", "init")
	terrInit.Dir = "./eks"
	out, _ := terrInit.StdoutPipe()
	terrInit.Start()
	scanInit := bufio.NewScanner(out)
	for scanInit.Scan() {
		m := scanInit.Text()
		fmt.Println(m)
	}

	terrInit.Wait()

	// Check if AWS authenticator binary is present in the working directory
	if _, err := exec.LookPath("aws-iam-authenticator"); err != nil {
		log.Fatalln("AWS Authenticator binary not found")
	}

	log.Println("starting terraform apply")
	terrSet := exec.Command("terraform", "apply", "-var-file=credentials.tfvars", "-auto-approve")
	terrSet.Dir = "./eks"
	stdout, err := terrSet.StdoutPipe()
	terrSet.Stderr = terrSet.Stdout
	terrSet.Start()

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		m := scanner.Text()
		fmt.Println(m)
	}

	terrSet.Wait()

	// Export KUBECONFIG file to the installation folder
	log.Println("Exporting kubeconfig file to the installation folder")

	kubeconf := exec.Command("terraform", "output", "kubeconfig")

	// open the out file for writing
	outfile, err := os.Create("./eks/kubeconfig")
	if err != nil {
		panic(err)
	}
	defer outfile.Close()
	kubeconf.Stdout = outfile

	err = kubeconf.Start()
	if err != nil {
		panic(err)
	}
	kubeconf.Wait()

	log.Println("To use the kubeconfig file, do the following:")

	log.Println("export KUBECONFIG=~/.kubeconfig")

	// Output configmap to create Worker nodes

	log.Println("Exporting Worker nodes config-map to the installation folder")

	confmap := exec.Command("terraform", "output", "config-map")

	// open the out file for writing
	outconf, err := os.Create("./eks/config-map-aws-auth.yaml")
	if err != nil {
		panic(err)
	}
	defer outconf.Close()
	confmap.Stdout = outconf

	err = confmap.Start()
	if err != nil {
		panic(err)
	}
	confmap.Wait()

	// Create Worker nodes usign the Configmap created above

	log.Println("Creating Worker Nodes")
	WorkerNodeSet := exec.Command("kubectl", "--kubeconfig", "./eks/kubeconfig", "apply", "-f", "./eks/config-map-aws-auth.yaml")
	WorkerNodeSet.Dir = "./eks"

	workerNodeOut, err := WorkerNodeSet.Output()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf(string(workerNodeOut))

	log.Println("Worker nodes are coming up one by one, it will take some time depending on the number of worker nodes you specified")

	os.Exit(0)

}

func EKSDestroy() {

	// Check if a terraform state file already exists
	if _, err := os.Stat("./eks/terraform.tfstate"); err != nil {
		log.Fatalln("Terraform.tfstate file not found, seems there is no existing cluster definition in this directory")
	}

	// Terraform destroy the EKS cluster

	log.Println("starting terraform destroy")

	terrDel := exec.Command("terraform", "destroy", "-force")
	terrDel.Dir = "./eks"
	out, _ := terrDel.StdoutPipe()
	terrDel.Start()
	scanDel := bufio.NewScanner(out)
	for scanDel.Scan() {
		m := scanDel.Text()
		fmt.Println(m)
	}

	terrDel.Wait()

	// Delete terraform state file

	log.Println("Removing the terraform state file")

	err := os.Remove("./eks/terraform.tfstate")
	if err != nil {
		fmt.Println(err)
	}
}
