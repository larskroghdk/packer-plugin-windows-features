#Requires -Version 5.1

<#
.SYNOPSIS
    Tests the guest servicing contract without changing Windows.

.DESCRIPTION
    Replaces servicing cmdlets with stubs and mocks, verifies interface dispatch, source options, restart suppression, structured errors and inventories.

.EXAMPLE
    Invoke-Pester -Path ./test/pester

.NOTES
    Author  : Lars Krogh
    Version : 1.0.0
#>


BeforeAll {
    . "$PSScriptRoot/../../guest/scripts/WindowsFeatures.Servicing.ps1"
    $script:commands = @('Get-WindowsFeature', 'Install-WindowsFeature', 'Uninstall-WindowsFeature', 'Get-WindowsOptionalFeature', 'Enable-WindowsOptionalFeature', 'Disable-WindowsOptionalFeature', 'Get-WindowsCapability', 'Add-WindowsCapability', 'Remove-WindowsCapability')
    foreach ($name in $commands) {
        Set-Item -Path "Function:global:$name" -Value {
            [CmdletBinding()]
            param($Name, $FeatureName, $Source, [switch]$Online, [switch]$All, [switch]$NoRestart, [switch]$Remove, [switch]$IncludeManagementTools, [switch]$IncludeAllSubFeature, [switch]$LimitAccess)
            throw ('An unmocked servicing command was called with ' + (@($Name, $FeatureName, $Source, $Online, $All, $NoRestart, $Remove, $IncludeManagementTools, $IncludeAllSubFeature, $LimitAccess) -join ', '))
        }
    }
}

AfterAll {
    foreach ($name in $commands) {
        Remove-Item -Path "Function:global:$name" -ErrorAction SilentlyContinue
    }
}

Describe 'Servicing operations' {
    BeforeEach {
        Mock Import-Module {}
        Mock Write-WFLog {}
        $script:request = [PSCustomObject]@{
            id = 'request-1'; interface = 'server_manager'; action = 'install'; name = 'Web-Server'
            source = 'D:\sources\sxs'; limit_access = $false; remove_payload = $false
            include_management_tools = $true; include_all_subfeatures = $true
        }
        Mock Install-WindowsFeature {
            [PSCustomObject]@{ Success = $true; RestartNeeded = 'Yes'; ExitCode = 'SuccessRestartRequired'; FeatureResult = @([PSCustomObject]@{Name = 'Web-Server' }) }
        }
        Mock Uninstall-WindowsFeature {
            [PSCustomObject]@{ Success = $true; RestartNeeded = 'No'; ExitCode = 'Success'; FeatureResult = @() }
        }
        Mock Enable-WindowsOptionalFeature { [PSCustomObject]@{RestartNeeded = $true } }
        Mock Disable-WindowsOptionalFeature { [PSCustomObject]@{RestartNeeded = $false } }
        Mock Add-WindowsCapability { [PSCustomObject]@{RestartNeeded = $false } }
        Mock Remove-WindowsCapability { [PSCustomObject]@{RestartNeeded = $true } }
    }
    It 'passes Server Manager options and reports the restart and changed names' {
        $result = Invoke-WFFeatureOperation $request
        $result.id | Should -Be 'request-1'
        $result.ok | Should -BeTrue
        $result.restart_needed | Should -BeTrue
        $result.changed | Should -Contain 'Web-Server'
        Should -Invoke Install-WindowsFeature -Times 1 -Exactly -ParameterFilter {
            $Name -eq 'Web-Server' -and $Source -eq 'D:\sources\sxs' -and $IncludeManagementTools -and $IncludeAllSubFeature
        }
    }
    It 'removes a server payload without passing an installation source' {
        $request.action = 'remove'
        $request.remove_payload = $true
        $result = Invoke-WFFeatureOperation $request
        $result.ok | Should -BeTrue
        $result.restart_needed | Should -BeFalse
        Should -Invoke Uninstall-WindowsFeature -Times 1 -Exactly -ParameterFilter { $Remove -and -not $Source }
    }
    It 'enables an optional feature with parents and suppresses automatic restart' {
        $request.interface = 'optional_feature'
        $request.name = 'NetFx3'
        $request.limit_access = $true
        (Invoke-WFFeatureOperation $request).restart_needed | Should -BeTrue
        Should -Invoke Enable-WindowsOptionalFeature -Times 1 -Exactly -ParameterFilter {
            $Online -and $All -and $NoRestart -and $LimitAccess -and $FeatureName -eq 'NetFx3' -and $Source -eq 'D:\sources\sxs'
        }
        Should -Invoke Install-WindowsFeature -Times 0 -Exactly
    }
    It 'disables optional features with payload removal and no restart' {
        $request.interface = 'optional_feature'
        $request.action = 'remove'
        $request.remove_payload = $true
        (Invoke-WFFeatureOperation $request).ok | Should -BeTrue
        Should -Invoke Disable-WindowsOptionalFeature -Times 1 -Exactly -ParameterFilter { $Online -and $NoRestart -and $Remove -and -not $Source }
    }
    It 'uses the capability namespace separately from optional features' {
        $request.interface = 'capability'
        $request.name = 'OpenSSH.Server~~~~0.0.1.0'
        (Invoke-WFFeatureOperation $request).ok | Should -BeTrue
        Should -Invoke Add-WindowsCapability -Times 1 -Exactly -ParameterFilter { $Online -and $Name -eq 'OpenSSH.Server~~~~0.0.1.0' }
        Should -Invoke Enable-WindowsOptionalFeature -Times 0 -Exactly
        $request.action = 'remove'
        (Invoke-WFFeatureOperation $request).restart_needed | Should -BeTrue
        Should -Invoke Remove-WindowsCapability -Times 1 -Exactly -ParameterFilter { $Online -and -not $Source }
    }
    It 'returns a structured error when servicing throws' {
        Mock Install-WindowsFeature { throw 'The source files could not be found. 0x800F081F' }
        $result = Invoke-WFFeatureOperation $request
        $result.ok | Should -BeFalse
        $result.id | Should -Be 'request-1'
        $result.error | Should -Match '0x800F081F'
        $result.exit_code | Should -Be 'Exception'
    }
    It 'does not turn a Server Manager failure into success' {
        Mock Install-WindowsFeature { [PSCustomObject]@{Success = $false; RestartNeeded = 'No'; ExitCode = 'Failed'; FeatureResult = @() } }
        $result = Invoke-WFFeatureOperation $request
        $result.ok | Should -BeFalse
        $result.error | Should -Match 'Failed'
    }
    It 'rejects unknown interfaces and actions' {
        $request.interface = 'unknown'
        (Invoke-WFFeatureOperation $request).ok | Should -BeFalse
        $request.action = 'unknown'
        { Invoke-WFFeatureOperation $request } | Should -Throw '*Unsupported action*'
    }
}

Describe 'Inventories' {
    BeforeEach { Mock Import-Module {} }
    It 'retains Server Manager dependencies and parents as arrays' {
        Mock Get-WindowsFeature {
            [PSCustomObject]@{Name = 'Web-WebServer'; DisplayName = 'Web Server'; InstallState = 'Installed'; FeatureType = 'Role Service'; DependsOn = 'WAS'; Parent = 'Web-Server'; SubFeatures = $null }
        }
        $result = Get-WFFeatureInventory server_manager
        $result.interface | Should -Be 'server_manager'
        $result.features.Count | Should -Be 1
        $result.features[0].depends_on.Count | Should -Be 1
        $result.features[0].depends_on[0] | Should -Be 'WAS'
        $result.features[0].sub_features.Count | Should -Be 0
        $result.features[0].parent | Should -Be 'Web-Server'
    }
    It 'maps optional feature names and states without Server Manager' {
        Mock Get-WindowsOptionalFeature { [PSCustomObject]@{FeatureName = 'NetFx3'; State = 'DisabledWithPayloadRemoved' } }
        $result = Get-WFFeatureInventory optional_feature
        $result.features[0].name | Should -Be 'NetFx3'
        $result.features[0].state | Should -Be 'DisabledWithPayloadRemoved'
    }
    It 'keeps full capability identities' {
        Mock Get-WindowsCapability { [PSCustomObject]@{Name = 'OpenSSH.Server~~~~0.0.1.0'; State = 'NotPresent' } }
        (Get-WFFeatureInventory capability).features[0].name | Should -Be 'OpenSSH.Server~~~~0.0.1.0'
    }
    It 'returns an empty array for an empty inventory' {
        Mock Get-WindowsCapability {}
        (Get-WFFeatureInventory capability).features.Count | Should -Be 0
    }
}
