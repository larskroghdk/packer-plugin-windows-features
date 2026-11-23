packer {
  required_plugins {
    windows-features = {
      source  = "github.com/larskroghdk/windows-features"
      version = ">= 0.1.0"
    }
  }
}

variable "host" { type = string }
variable "username" {
  type    = string
  default = "Administrator"
}
variable "password" {
  type      = string
  sensitive = true
}
variable "use_ssl" {
  type    = bool
  default = true
}
variable "insecure" {
  type    = bool
  default = false
}
variable "dry_run" {
  type    = bool
  default = true
}

source "null" "windows" {
  communicator   = "winrm"
  winrm_host     = var.host
  winrm_username = var.username
  winrm_password = var.password
  winrm_use_ssl  = var.use_ssl
  winrm_insecure = var.insecure
  winrm_use_ntlm = true
  winrm_timeout  = "5m"
}

build {
  sources = ["source.null.windows"]
  provisioner "windows-features" {
    install             = ["Microsoft-Hyper-V-All"]
    dry_run             = var.dry_run
    restart_if_required = true
    report_path         = "reports/client-hyperv.json"
  }
}
