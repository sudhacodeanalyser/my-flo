#!/usr/bin/python3

import subprocess
import os

FLO_HELM_REPO_NAME = 'flo-helm-charts'
FLO_HELM_REPO_URL  = 'https://nexus.flotech.co/repository/helm-charts/'

out = subprocess.run(["helm", "repo", "list"], stdout=subprocess.PIPE, text=True).stdout
out_lines = out.split("\n")


def flo_repo_enabled(helm_stdout):
    return FLO_HELM_REPO_NAME in [repo.split()[0] for repo in helm_stdout[1:] if len(repo.split()) > 1]


if len(out_lines) < 2 or not flo_repo_enabled(out_lines):
    subprocess.run(["helm", "repo", "add", FLO_HELM_REPO_NAME, FLO_HELM_REPO_URL])

tpl_s = os.system("helm template " + FLO_HELM_REPO_NAME + "/consul --namespace consul --values values.yaml | tee output.yaml")
if tpl_s != 0:
    exit(tpl_s)

os.system("helm upgrade --install consul " + FLO_HELM_REPO_NAME + "/consul --wait --namespace consul --values values.yaml")
