#Requires -Version 5.1

<#
.SYNOPSIS
    Reads the pending restart indicators and the boot time.

.DESCRIPTION
    Dot-sourced by Invoke-WindowsFeatures.ps1. All registry reads go through the 64-bit view
    of HKEY_LOCAL_MACHINE, so they are not redirected by WOW64.

.EXAMPLE
    . .\WindowsFeatures.Reboot.ps1

.NOTES
    Author  : Lars Krogh
    Version : 1.0.0
#>


function Test-WFRegistryKey {
    <#
    .SYNOPSIS
        Tests whether a registry key exists below a root key.

    .DESCRIPTION
        Opens the key read-only and disposes it again. A missing key returns $false.

    .PARAMETER Root
        The open root key, normally the 64-bit HKEY_LOCAL_MACHINE view.

    .PARAMETER Path
        The key path below the root, without a leading backslash.

    .EXAMPLE
        Test-WFRegistryKey -Root $hklm -Path 'SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending'

    .OUTPUTS
        System.Boolean

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    [OutputType([bool])]
    param(
        [Parameter(Mandatory)]
        [Microsoft.Win32.RegistryKey]$Root,
        [Parameter(Mandatory)]
        [string]$Path
    )
    $key = $Root.OpenSubKey($Path, $false)
    if ($null -eq $key) {
        return $false
    }
    $key.Dispose()
    $true
}

function Get-WFRegistryValue {
    <#
    .SYNOPSIS
        Reads one registry value below a root key.

    .DESCRIPTION
        Returns the value data, or $null when the key or the value does not exist.

    .PARAMETER Root
        The open root key, normally the 64-bit HKEY_LOCAL_MACHINE view.

    .PARAMETER Path
        The key path below the root, without a leading backslash.

    .PARAMETER Name
        The value name.

    .EXAMPLE
        Get-WFRegistryValue -Root $hklm -Path 'SYSTEM\CurrentControlSet\Control\ComputerName\ComputerName' -Name ComputerName

    .OUTPUTS
        System.Object

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [Microsoft.Win32.RegistryKey]$Root,
        [Parameter(Mandatory)]
        [string]$Path,
        [Parameter(Mandatory)]
        [string]$Name
    )
    $key = $Root.OpenSubKey($Path, $false)
    if ($null -eq $key) {
        return $null
    }
    try {
        $key.GetValue($Name, $null)
    }
    finally {
        $key.Dispose()
    }
}

function Get-WFPendingReboot {
    <#
    .SYNOPSIS
        Reads the indicators Windows sets when a restart is pending.

    .DESCRIPTION
        Checks the Component Based Servicing RebootPending key, the Windows Update
        RebootRequired key, non-empty PendingFileRenameOperations, and a difference between
        the active and the configured computer name. The plugin decides which indicators
        block servicing.

    .PARAMETER Root
        The root key to read from. Defaults to the 64-bit HKEY_LOCAL_MACHINE view. Tests pass
        a scratch key.

    .EXAMPLE
        (Get-WFPendingReboot).component_based_servicing

    .OUTPUTS
        System.Management.Automation.PSCustomObject

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter()]
        [Microsoft.Win32.RegistryKey]$Root
    )
    $owned = $null -eq $Root
    if ($owned) {
        $Root = [Microsoft.Win32.RegistryKey]::OpenBaseKey([Microsoft.Win32.RegistryHive]::LocalMachine, [Microsoft.Win32.RegistryView]::Registry64)
    }
    try {
        $renames = @(Get-WFRegistryValue -Root $Root -Path 'SYSTEM\CurrentControlSet\Control\Session Manager' -Name 'PendingFileRenameOperations' |
                Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
        $active = Get-WFRegistryValue -Root $Root -Path 'SYSTEM\CurrentControlSet\Control\ComputerName\ActiveComputerName' -Name 'ComputerName'
        $configured = Get-WFRegistryValue -Root $Root -Path 'SYSTEM\CurrentControlSet\Control\ComputerName\ComputerName' -Name 'ComputerName'
        [PSCustomObject]@{
            component_based_servicing = Test-WFRegistryKey -Root $Root -Path 'SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending'
            windows_update            = Test-WFRegistryKey -Root $Root -Path 'SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired'
            pending_file_rename       = $renames.Count -gt 0
            computer_rename           = ($null -ne $active) -and ($null -ne $configured) -and ($active -ne $configured)
        }
    }
    finally {
        if ($owned) {
            $Root.Dispose()
        }
    }
}

function Get-WFBootTime {
    <#
    .SYNOPSIS
        Returns the last boot time as a UTC ISO 8601 string.

    .DESCRIPTION
        Reads LastBootUpTime from Win32_OperatingSystem. The plugin compares it before and
        after a restart to know that the machine really restarted.

    .EXAMPLE
        Get-WFBootTime

    .OUTPUTS
        System.String

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    [OutputType([string])]
    param()
    $os = Get-CimInstance -ClassName Win32_OperatingSystem -ErrorAction Stop
    $os.LastBootUpTime.ToUniversalTime().ToString('o', [Globalization.CultureInfo]::InvariantCulture)
}
