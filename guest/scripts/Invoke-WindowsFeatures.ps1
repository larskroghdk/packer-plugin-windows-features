#Requires -Version 5.1

<#
.SYNOPSIS
    Runs one windows-features guest action and writes its result as JSON.

.DESCRIPTION
    Entry point that the Packer plugin starts over WinRM, once per action. It loads the
    WindowsFeatures.*.ps1 parts from its own directory, reads the JSON request from
    -InputPath when the action takes one, and writes the JSON result to -OutputPath.
    The script only detects, reads and changes state. Every decision is made by the plugin.
    Detect     collects operating system, session, servicing, restart and WinRM facts.
    Probe      re-reads WinRM, the session, the boot time and the restart state.
    Inventory  lists every feature or capability of one servicing interface with its state.
    Invoke     installs or removes one feature or capability.
    Exits 0 when the action ran, even if the operation failed; Invoke reports that in its
    result. Exits 1 when the action itself failed, with the error on stderr.

.PARAMETER Action
    The action to run.

.PARAMETER InputPath
    JSON request file. Required by Inventory and Invoke.

.PARAMETER OutputPath
    Where the JSON result is written.

.EXAMPLE
    powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\Invoke-WindowsFeatures.ps1 -Action Detect -OutputPath C:\Windows\Temp\wf\01-detect-output.json

.EXAMPLE
    powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\Invoke-WindowsFeatures.ps1 -Action Inventory -InputPath C:\Windows\Temp\wf\02-inventory-input.json -OutputPath C:\Windows\Temp\wf\02-inventory-output.json

.OUTPUTS
    None. The result is written to -OutputPath. Progress lines go to stdout.

.NOTES
    Author  : Lars Krogh
    Version : 1.0.0
#>

[CmdletBinding()]
param (
    [Parameter(Mandatory)]
    [ValidateSet('Detect', 'Probe', 'Inventory', 'Invoke')]
    [string]$Action,
    [Parameter()]
    [string]$InputPath,
    [Parameter(Mandatory)]
    [string]$OutputPath
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

trap {
    [Console]::Error.WriteLine("ERROR: $($_.Exception.Message)")
    [Console]::Error.WriteLine("ERROR: $($_.ScriptStackTrace)")
    exit 1
}

$scriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
foreach ($part in 'Facts', 'Reboot', 'Servicing') {
    . (Join-Path -Path $scriptRoot -ChildPath "WindowsFeatures.$part.ps1")
}

function Read-WFInput {
    <#
    .SYNOPSIS
        Reads the JSON request that the plugin uploaded for the current action.

    .DESCRIPTION
        Parses the file as UTF-8 JSON and returns the request object.

    .PARAMETER Path
        The request file, the script's -InputPath. Empty when the plugin did not send one.

    .EXAMPLE
        $request = Read-WFInput -Path $InputPath

    .OUTPUTS
        System.Management.Automation.PSCustomObject

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter()]
        [AllowEmptyString()]
        [string]$Path
    )
    if ([string]::IsNullOrEmpty($Path)) {
        throw 'This action requires -InputPath'
    }
    [IO.File]::ReadAllText($Path, [Text.Encoding]::UTF8) | ConvertFrom-Json
}

function Write-WFResult {
    <#
    .SYNOPSIS
        Writes the action result to the output file as JSON.

    .DESCRIPTION
        Serializes the result to compact JSON and writes it as UTF-8 without a byte order
        mark. The plugin downloads and decodes this file.

    .PARAMETER Path
        The result file, the script's -OutputPath.

    .PARAMETER Result
        The object to serialize. Property names are the JSON field names the Go side decodes.

    .EXAMPLE
        Write-WFResult -Path $OutputPath -Result (Get-WFSystemFact)

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param (
        [Parameter(Mandatory)]
        [string]$Path,
        [Parameter(Mandatory)]
        [object]$Result
    )
    $json = ConvertTo-Json -InputObject $Result -Depth 10 -Compress
    [IO.File]::WriteAllText($Path, $json, (New-Object System.Text.UTF8Encoding ($false)))
}

switch ($Action) {
    'Detect' {
        Write-WFResult -Path $OutputPath -Result (Get-WFSystemFact)
    }
    'Probe' {
        Write-WFResult -Path $OutputPath -Result (Get-WFProbe)
    }
    'Inventory' {
        $request = Read-WFInput -Path $InputPath
        Write-WFResult -Path $OutputPath -Result (Get-WFFeatureInventory -Interface $request.interface)
    }
    'Invoke' {
        $request = Read-WFInput -Path $InputPath
        Write-WFResult -Path $OutputPath -Result (Invoke-WFFeatureOperation -Request $request)
    }
}
exit 0
