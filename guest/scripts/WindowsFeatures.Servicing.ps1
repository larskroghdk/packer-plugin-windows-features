#Requires -Version 5.1

<#
.SYNOPSIS
    Lists, installs and removes Windows features, optional features and capabilities.

.DESCRIPTION
    Dot-sourced by Invoke-WindowsFeatures.ps1. Each servicing interface uses its own
    supported cmdlets: the ServerManager module for Windows Server roles and features, and
    the Dism module for optional features and capabilities. States are returned as the
    cmdlets report them; the plugin normalizes and judges them. Restarts are never
    started here: every operation runs with its no-restart behavior.

.EXAMPLE
    . .\WindowsFeatures.Servicing.ps1

.NOTES
    Author  : Lars Krogh
    Version : 1.0.0
#>


function Write-WFLog {
    <#
    .SYNOPSIS
        Writes a progress line to standard output.

    .DESCRIPTION
        Writes the message straight to the console output stream so that Packer shows it in
        the build log. It bypasses the PowerShell pipeline, so the line never ends up in a
        function's return value.

    .PARAMETER Message
        The line to write.

    .EXAMPLE
        Write-WFLog 'Installed Web-Server'

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]$Message
    )
    [Console]::Out.WriteLine($Message)
}

function ConvertTo-WFStringArray {
    <#
    .SYNOPSIS
        Converts a cmdlet property that may be null, a string or a list to a string array.

    .DESCRIPTION
        Removes null and empty entries, so the JSON always has an array of names.

    .PARAMETER Value
        The property value.

    .EXAMPLE
        ConvertTo-WFStringArray -Value $feature.DependsOn

    .OUTPUTS
        System.String[]

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    [OutputType([System.Array])]
    param(
        [Parameter()]
        [AllowNull()]
        [object]$Value
    )
    $out = @($Value | Where-Object { -not [string]::IsNullOrEmpty([string]$_) } | ForEach-Object { [string]$_ })
    , $out
}

function Get-WFServerFeatureInventory {
    <#
    .SYNOPSIS
        Lists every Windows Server role, role service and feature.

    .DESCRIPTION
        Calls Get-WindowsFeature and returns the name, display name, install state, feature
        type, dependencies, parent and sub-features of each entry. The plugin uses the
        dependencies to compute what an install adds and what a removal takes away.

    .EXAMPLE
        Get-WFServerFeatureInventory | Where-Object state -eq 'Installed'

    .OUTPUTS
        System.Management.Automation.PSCustomObject

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param()
    Import-Module -Name ServerManager -ErrorAction Stop -Verbose:$false
    foreach ($feature in (Get-WindowsFeature -ErrorAction Stop)) {
        [PSCustomObject]@{
            name         = [string]$feature.Name
            display_name = [string]$feature.DisplayName
            state        = [string]$feature.InstallState
            type         = [string]$feature.FeatureType
            depends_on   = ConvertTo-WFStringArray -Value $feature.DependsOn
            parent       = [string]$feature.Parent
            sub_features = ConvertTo-WFStringArray -Value $feature.SubFeatures
        }
    }
}

function Get-WFOptionalFeatureInventory {
    <#
    .SYNOPSIS
        Lists every optional feature of the running Windows.

    .DESCRIPTION
        Calls Get-WindowsOptionalFeature -Online, which lists names and states but no
        display names or dependencies.

    .EXAMPLE
        Get-WFOptionalFeatureInventory | Where-Object state -eq 'Enabled'

    .OUTPUTS
        System.Management.Automation.PSCustomObject

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param()
    foreach ($feature in (Get-WindowsOptionalFeature -Online -ErrorAction Stop)) {
        [PSCustomObject]@{
            name         = [string]$feature.FeatureName
            display_name = ''
            state        = [string]$feature.State
            type         = ''
            depends_on   = @()
            parent       = ''
            sub_features = @()
        }
    }
}

function Get-WFCapabilityInventory {
    <#
    .SYNOPSIS
        Lists every Windows capability (Feature on Demand) of the running Windows.

    .DESCRIPTION
        Calls Get-WindowsCapability -Online. Capability names carry a version suffix, for
        example OpenSSH.Server~~~~0.0.1.0.

    .EXAMPLE
        Get-WFCapabilityInventory | Where-Object state -eq 'Installed'

    .OUTPUTS
        System.Management.Automation.PSCustomObject

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param()
    foreach ($capability in (Get-WindowsCapability -Online -ErrorAction Stop)) {
        [PSCustomObject]@{
            name         = [string]$capability.Name
            display_name = ''
            state        = [string]$capability.State
            type         = ''
            depends_on   = @()
            parent       = ''
            sub_features = @()
        }
    }
}

function Get-WFFeatureInventory {
    <#
    .SYNOPSIS
        Lists every entry of one servicing interface for the Inventory action.

    .DESCRIPTION
        Dispatches to the inventory function of the interface and wraps the entries with the
        interface name.

    .PARAMETER Interface
        server_manager, optional_feature or capability.

    .EXAMPLE
        Get-WFFeatureInventory -Interface server_manager

    .OUTPUTS
        System.Management.Automation.PSCustomObject with interface and features.

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [ValidateSet('server_manager', 'optional_feature', 'capability')]
        [string]$Interface
    )
    $features = switch ($Interface) {
        'server_manager' { Get-WFServerFeatureInventory }
        'optional_feature' { Get-WFOptionalFeatureInventory }
        'capability' { Get-WFCapabilityInventory }
    }
    [PSCustomObject]@{
        interface = $Interface
        features  = @($features)
    }
}

function Invoke-WFServerFeatureOperation {
    <#
    .SYNOPSIS
        Installs or uninstalls one Windows Server role, role service or feature.

    .DESCRIPTION
        Uses Install-WindowsFeature or Uninstall-WindowsFeature without -Restart, so Windows
        never restarts here. Server Manager adds required dependencies on install and removes
        dependent features on uninstall.

    .PARAMETER Request
        The operation request sent by the plugin.

    .EXAMPLE
        Invoke-WFServerFeatureOperation -Request $request

    .OUTPUTS
        System.Management.Automation.PSCustomObject with ok, restart_needed, exit_code and changed.

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [object]$Request
    )
    Import-Module -Name ServerManager -ErrorAction Stop -Verbose:$false
    $params = @{ Name = $Request.name; ErrorAction = 'Stop'; WarningAction = 'SilentlyContinue' }
    if ($Request.include_management_tools) {
        $params.IncludeManagementTools = $true
    }
    if ($Request.action -eq 'install') {
        if ($Request.include_all_subfeatures) {
            $params.IncludeAllSubFeature = $true
        }
        if (-not [string]::IsNullOrEmpty($Request.source)) {
            $params.Source = $Request.source
        }
        $result = Install-WindowsFeature @params
    }
    else {
        if ($Request.remove_payload) {
            $params.Remove = $true
        }
        $result = Uninstall-WindowsFeature @params
    }
    $exitCode = [string]$result.ExitCode
    [PSCustomObject]@{
        ok             = [bool]$result.Success
        restart_needed = ([string]$result.RestartNeeded) -ne 'No'
        exit_code      = $exitCode
        changed        = ConvertTo-WFStringArray -Value @($result.FeatureResult | ForEach-Object { $_.Name })
        error          = if ([bool]$result.Success) { $null } else { "Server Manager reported exit code $exitCode" }
    }
}

function Invoke-WFDismOperation {
    <#
    .SYNOPSIS
        Enables or disables one optional feature, or adds or removes one capability.

    .DESCRIPTION
        Uses Enable-WindowsOptionalFeature -All -NoRestart, Disable-WindowsOptionalFeature
        -NoRestart, Add-WindowsCapability or Remove-WindowsCapability against the running
        Windows. -All enables the parent features an optional feature needs, which is how
        DISM resolves dependencies. -Source and -LimitAccess are passed only on install.

    .PARAMETER Request
        The operation request sent by the plugin.

    .EXAMPLE
        Invoke-WFDismOperation -Request $request

    .OUTPUTS
        System.Management.Automation.PSCustomObject with ok, restart_needed, exit_code and changed.

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [object]$Request
    )
    $params = @{ Online = $true; ErrorAction = 'Stop'; WarningAction = 'SilentlyContinue' }
    $install = $Request.action -eq 'install'
    if ($install -and -not [string]::IsNullOrEmpty($Request.source)) {
        $params.Source = $Request.source
    }
    if ($install -and $Request.limit_access) {
        $params.LimitAccess = $true
    }
    if ($Request.interface -eq 'optional_feature') {
        $params.FeatureName = $Request.name
        $params.NoRestart = $true
        if ($install) {
            $params.All = $true
            $result = Enable-WindowsOptionalFeature @params
        }
        else {
            if ($Request.remove_payload) {
                $params.Remove = $true
            }
            $result = Disable-WindowsOptionalFeature @params
        }
    }
    else {
        $params.Name = $Request.name
        if ($install) {
            $result = Add-WindowsCapability @params
        }
        else {
            $result = Remove-WindowsCapability @params
        }
    }
    $restart = $false
    foreach ($item in @($result)) {
        if ($null -ne $item -and $null -ne $item.PSObject.Properties['RestartNeeded'] -and [bool]$item.RestartNeeded) {
            $restart = $true
        }
    }
    [PSCustomObject]@{
        ok             = $true
        restart_needed = $restart
        exit_code      = 'Success'
        changed        = @($Request.name)
        error          = $null
    }
}

function Invoke-WFFeatureOperation {
    <#
    .SYNOPSIS
        Runs one install or remove operation for the Invoke action.

    .DESCRIPTION
        Dispatches on the interface and catches every failure, so the result always reports
        what happened: ok, the restart flag, the cmdlet's exit code, the features it touched
        and, on failure, the error message with its HRESULT. The plugin verifies the outcome
        by reading the inventory again; this result is only evidence.

    .PARAMETER Request
        The operation request: id, interface, action, name and the options.

    .EXAMPLE
        Invoke-WFFeatureOperation -Request (Read-WFInput -Path $InputPath)

    .OUTPUTS
        System.Management.Automation.PSCustomObject with id, ok, restart_needed, exit_code, changed and error.

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [object]$Request
    )
    if (@('install', 'remove') -notcontains $Request.action) {
        throw "Unsupported action '$($Request.action)'"
    }
    try {
        switch ($Request.interface) {
            'server_manager' { $result = Invoke-WFServerFeatureOperation -Request $Request }
            'optional_feature' { $result = Invoke-WFDismOperation -Request $Request }
            'capability' { $result = Invoke-WFDismOperation -Request $Request }
            default { throw "Unsupported interface '$($Request.interface)'" }
        }
        Write-WFLog "$($Request.action) $($Request.name): exit code $($result.exit_code), restart needed $($result.restart_needed)"
    }
    catch {
        $message = $_.Exception.Message
        $hresult = $_.Exception.HResult
        if ($hresult -ne 0) {
            $message = '{0} (HRESULT 0x{1:X8})' -f $message, $hresult
        }
        Write-WFLog "$($Request.action) $($Request.name) failed: $message"
        $result = [PSCustomObject]@{
            ok             = $false
            restart_needed = $false
            exit_code      = 'Exception'
            changed        = @()
            error          = $message
        }
    }
    [PSCustomObject]@{
        id             = $Request.id
        ok             = [bool]$result.ok
        restart_needed = [bool]$result.restart_needed
        exit_code      = [string]$result.exit_code
        changed        = @($result.changed)
        error          = $result.error
    }
}
