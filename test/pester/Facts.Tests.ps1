#Requires -Version 5.1

<#
.SYNOPSIS
    Tests OS, probe and restart fact contracts without altering the host.

.DESCRIPTION
    Mocks registry, CIM and WinRM reads and checks that collection failures remain visible.

.EXAMPLE
    Invoke-Pester -Path ./test/pester

.NOTES
    Author  : Lars Krogh
    Version : 1.0.0
#>


BeforeAll {
    . "$PSScriptRoot/../../guest/scripts/WindowsFeatures.Facts.ps1"
    . "$PSScriptRoot/../../guest/scripts/WindowsFeatures.Reboot.ps1"
}

Describe 'Structured Windows facts' {
    It 'keeps product type and build even when a Windows 11 product name says Windows 10' {
        Mock Get-ItemProperty {
            [PSCustomObject]@{ProductName = 'Windows 10 Enterprise'; EditionID = 'Enterprise'; InstallationType = 'Client'; CurrentMajorVersionNumber = 10; CurrentMinorVersionNumber = 0; CurrentBuildNumber = '26100'; UBR = 1000 }
        }
        Mock Get-CimInstance { [PSCustomObject]@{ProductType = 1 } }
        $result = Get-WFOperatingSystem
        $result.build | Should -Be 26100
        $result.product_type | Should -Be 1
        $result.product_name | Should -Be 'Windows 10 Enterprise'
        $result.display_version | Should -BeNullOrEmpty
    }

    It 'converts WSMan false strings to false rather than treating a nonempty string as true' {
        (ConvertTo-WFBoolean 'false') | Should -BeFalse
        (ConvertTo-WFBoolean 'TRUE') | Should -BeTrue
        (ConvertTo-WFBoolean $null) | Should -BeFalse
    }

    It 'retains failed fact collection as null with an error' {
        Mock Get-WFOperatingSystem { [PSCustomObject]@{build = 20348 } }
        Mock Get-WFSessionIdentity { [PSCustomObject]@{is_elevated = $true } }
        Mock Get-WFServicingSupport { [PSCustomObject]@{server_manager = $true } }
        Mock Get-WFPendingReboot { throw 'restart registry unavailable' }
        Mock Get-WFWinRMConfiguration { throw 'WinRM unavailable' }
        $result = Get-WFSystemFact
        $result.reboot | Should -BeNullOrEmpty
        $result.winrm | Should -BeNullOrEmpty
        $result.errors.Count | Should -Be 2
    }

    It 'reports probe failures without losing boot time' {
        Mock Get-WFSessionIdentity { [PSCustomObject]@{is_elevated = $true } }
        Mock Get-WFPendingReboot { throw 'restart registry unavailable' }
        Mock Get-WFWinRMConfiguration { throw 'WinRM unavailable' }
        Mock Get-WFBootTime { '2026-01-01T00:00:00Z' }
        $result = Get-WFProbe
        $result.boot_time | Should -Be '2026-01-01T00:00:00Z'
        $result.error | Should -Match 'WinRM unavailable'
        $result.error | Should -Match 'restart registry unavailable'
    }

    It 'reads pending indicators through helpers and ignores empty rename entries' {
        Mock Test-WFRegistryKey { $Path -like '*RebootPending' }
        Mock Get-WFRegistryValue {
            if ($Name -eq 'PendingFileRenameOperations') { return @('', ' ') }
            if ($Path -like '*ActiveComputerName') { return 'BEFORE' }
            'AFTER'
        }
        $result = Get-WFPendingReboot -Root ([Microsoft.Win32.Registry]::CurrentUser)
        $result.component_based_servicing | Should -BeTrue
        $result.windows_update | Should -BeFalse
        $result.pending_file_rename | Should -BeFalse
        $result.computer_rename | Should -BeTrue
    }
}
