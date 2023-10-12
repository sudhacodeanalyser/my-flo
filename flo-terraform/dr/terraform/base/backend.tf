terraform {
  backend "s3" {
    region = "us-east-2"
    bucket = "k8s-dr-terraform-state"
    profile = "flo-dev"
    key    = "us-east-2/dr/dr/base.tfstate"
  }
}
provider "aws" {
  profile = "flo-dev"
  region = "us-east-2"
}
