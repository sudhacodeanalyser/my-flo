variable "vpc_name" {
  type = string
  default = "k8s-flo-k8s-flo-project-vpc" # k8s vpc
}

variable "k8s_masters_instances_names" {
  type = list
  default = ["master-us-west-2b.masters.k8s.flocloud.co", "master-us-west-2a.masters.k8s.flocloud.co", "master-us-west-2c.masters.k8s.flocloud.co"]
}