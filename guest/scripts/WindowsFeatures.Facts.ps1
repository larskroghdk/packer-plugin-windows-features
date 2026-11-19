#Requires -Version 5.1

<#
.SYNOPSIS
    Collects the guest facts that the plugin uses to select a servicing interface and protect WinRM.

.DESCRIPTION
    Dot-sourced by Invoke-WindowsFeatures.ps1. Reads the operating system, the session, the
    servicing cmdlets the guest offers, the pending restart state, and the WinRM
    configuration with the services WinRM depends on.

.EXAMPLE
    . .\WindowsFeatures.Facts.ps1

.NOTES
    Author  : Lars Krogh
    Version : 1.0.0
#>


function Get-WFOperatingSystem {
    <#
    .SYNOPSIS
        Collects the Windows version, edition and installation type.

    .DESCRIPTION
        Combines the CurrentVersion registry key with Win32_OperatingSystem. Values missing
        from the registry are returned as null or 0, so the plugin can reject an unknown
        release instead of guessing.

    .EXAMPLE
        (Get-WFOperatingSystem).build

    .OUTPUTS
        System.Management.Automation.PSCustomObject

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param()
    $currentVersion = Get-ItemProperty -LiteralPath 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion'
    $os = Get-CimInstance -ClassName Win32_OperatingSystem -ErrorAction Stop
    $optional = {
        param($name)
        $property = $currentVersion.PSObject.Properties[$name]
        if ($null -ne $property) { $property.Value } else { $null }
    }
    [PSCustomObject]@{
        product_name      = & $optional 'ProductName'
        edition_id        = & $optional 'EditionID'
        installation_type = & $optional 'InstallationType'
        display_version   = & $optional 'DisplayVersion'
        major             = [int](& $optional 'CurrentMajorVersionNumber')
        minor             = [int](& $optional 'CurrentMinorVersionNumber')
        build             = [int](& $optional 'CurrentBuildNumber')
        ubr               = [int](& $optional 'UBR')
        product_type      = [int]$os.ProductType
        architecture      = [string]$env:PROCESSOR_ARCHITECTURE
        is_64bit_process  = [Environment]::Is64BitProcess
    }
}

function Get-WFSessionIdentity {
    <#
    .SYNOPSIS
        Describes the account and elevation of the current WinRM session.

    .DESCRIPTION
        Returns the user name and whether the token holds the Administrators group. The
        plugin refuses to run without elevation and checks after every change that the
        session is still elevated.

    .EXAMPLE
        (Get-WFSessionIdentity).is_elevated

    .OUTPUTS
        System.Management.Automation.PSCustomObject

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param()
    $identity = [System.Security.Principal.WindowsIdentity]::GetCurrent()
    try {
        $principal = New-Object System.Security.Principal.WindowsPrincipal ($identity)
        [PSCustomObject]@{
            user_name   = $identity.Name
            is_elevated = $principal.IsInRole([System.Security.Principal.WindowsBuiltInRole]::Administrator)
        }
    }
    finally {
        $identity.Dispose()
    }
}

function ConvertTo-WFBoolean {
    <#
    .SYNOPSIS
        Converts a WSMan provider value to a Boolean.

    .DESCRIPTION
        The WSMan: drive reports settings as the strings 'true' and 'false'. Only 'true'
        (case-insensitive) converts to $true. Anything else, including an empty string,
        converts to $false.

    .PARAMETER Value
        The string value read from the WSMan: drive.

    .EXAMPLE
        ConvertTo-WFBoolean -Value (Get-Item WSMan:\localhost\Service\AllowUnencrypted).Value

    .OUTPUTS
        System.Boolean

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    [OutputType([bool])]
    param(
        [Parameter()]
        [AllowEmptyString()]
        [AllowNull()]
        [string]$Value
    )
    $Value -eq 'true'
}

function Get-WFServiceDependency {
    <#
    .SYNOPSIS
        Lists every service and driver a service depends on, directly or indirectly.

    .DESCRIPTION
        Walks ServicesDependedOn breadth-first and returns each service name once, sorted.
        The plugin never changes the start type of a service in this list.

    .PARAMETER Name
        The service whose dependencies are listed.

    .EXAMPLE
        Get-WFServiceDependency -Name WinRM

    .OUTPUTS
        System.String[]

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    [OutputType([string[]])]
    param(
        [Parameter(Mandatory)]
        [string]$Name
    )
    $seen = New-Object 'System.Collections.Generic.HashSet[string]' ([StringComparer]::OrdinalIgnoreCase)
    $queue = New-Object System.Collections.Generic.Queue[object]
    $queue.Enqueue((Get-Service -Name $Name -ErrorAction Stop))
    while ($queue.Count -gt 0) {
        $service = $queue.Dequeue()
        foreach ($dependency in $service.ServicesDependedOn) {
            if ($seen.Add($dependency.ServiceName)) {
                $queue.Enqueue($dependency)
            }
        }
    }
    $seen | Sort-Object
}

function Get-WFWinRMConfiguration {
    <#
    .SYNOPSIS
        Reads the WinRM service, listener and authentication configuration.

    .DESCRIPTION
        Returns the service state, the services WinRM depends on, every listener, whether
        unencrypted traffic is allowed and which authentication methods are enabled. The
        plugin compares this before and after changes to confirm that WinRM did not regress.

    .EXAMPLE
        (Get-WFWinRMConfiguration).auth.basic

    .OUTPUTS
        System.Management.Automation.PSCustomObject

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param()
    $service = Get-Service -Name WinRM -ErrorAction Stop
    $listeners = foreach ($listener in (Get-ChildItem -Path 'WSMan:\localhost\Listener' -ErrorAction Stop)) {
        $values = @{}
        foreach ($item in (Get-ChildItem -Path "WSMan:\localhost\Listener\$($listener.Name)" -ErrorAction Stop)) {
            $values[$item.Name] = [string]$item.Value
        }
        [PSCustomObject]@{
            transport = $values['Transport']
            address   = $values['Address']
            port      = [int]$values['Port']
            enabled   = ConvertTo-WFBoolean -Value $values['Enabled']
        }
    }
    $auth = @{}
    foreach ($item in (Get-ChildItem -Path 'WSMan:\localhost\Service\Auth' -ErrorAction Stop)) {
        $auth[$item.Name] = [string]$item.Value
    }
    [PSCustomObject]@{
        service_status    = $service.Status.ToString()
        service_start     = $service.StartType.ToString()
        dependencies      = @(Get-WFServiceDependency -Name WinRM)
        listeners         = @($listeners)
        allow_unencrypted = ConvertTo-WFBoolean -Value ([string](Get-Item -Path 'WSMan:\localhost\Service\AllowUnencrypted').Value)
        auth              = [PSCustomObject]@{
            basic       = ConvertTo-WFBoolean -Value $auth['Basic']
            kerberos    = ConvertTo-WFBoolean -Value $auth['Kerberos']
            negotiate   = ConvertTo-WFBoolean -Value $auth['Negotiate']
            certificate = ConvertTo-WFBoolean -Value $auth['Certificate']
            credssp     = ConvertTo-WFBoolean -Value $auth['CredSSP']
        }
    }
}

function Get-WFServicingSupport {
    <#
    .SYNOPSIS
        Reports which feature servicing cmdlets the guest has.

    .DESCRIPTION
        Looks for the cmdlets themselves instead of inferring them from the product name.
        Get-WindowsFeature comes with the ServerManager module on Windows Server,
        Get-WindowsOptionalFeature and Get-WindowsCapability with the Dism module.

    .EXAMPLE
        (Get-WFServicingSupport).server_manager

    .OUTPUTS
        System.Management.Automation.PSCustomObject with server_manager, optional_features and capabilities.

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param()
    $has = {
        param($name)
        $null -ne (Get-Command -Name $name -CommandType Cmdlet, Function -ErrorAction SilentlyContinue)
    }
    [PSCustomObject]@{
        server_manager    = & $has 'Get-WindowsFeature'
        optional_features = & $has 'Get-WindowsOptionalFeature'
        capabilities      = & $has 'Get-WindowsCapability'
    }
}

function Get-WFSystemFact {
    <#
    .SYNOPSIS
        Collects all guest facts for the Detect action.

    .DESCRIPTION
        Gathers the operating system, the session, the servicing cmdlets, the pending restart
        state and WinRM. When WinRM or restart detection fails, that part is null and the
        error is added to the errors list, so the plugin treats it as unknown and fails
        closed. Operating system and session failures stop the action.

    .EXAMPLE
        Write-WFResult -Path $OutputPath -Result (Get-WFSystemFact)

    .OUTPUTS
        System.Management.Automation.PSCustomObject

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param()
    $errors = New-Object System.Collections.Generic.List[string]
    $collect = {
        param($name, [scriptblock]$block)
        try {
            & $block
        }
        catch {
            $errors.Add("${name}: $($_.Exception.Message)")
            $null
        }
    }
    [PSCustomObject]@{
        os        = Get-WFOperatingSystem
        session   = Get-WFSessionIdentity
        servicing = Get-WFServicingSupport
        reboot    = & $collect 'reboot' { Get-WFPendingReboot }
        winrm     = & $collect 'winrm' { Get-WFWinRMConfiguration }
        errors    = @($errors)
    }
}

function Get-WFProbe {
    <#
    .SYNOPSIS
        Re-reads WinRM, the session, the boot time and the restart state for the Probe action.

    .DESCRIPTION
        Runs as a new WinRM command after changes and restarts. A WinRM read failure is
        returned in the error field instead of failing the action, so the plugin can report
        it as a regression. The boot time is UTC in ISO 8601 form; the plugin detects a
        restart when it changes.

    .EXAMPLE
        Write-WFResult -Path $OutputPath -Result (Get-WFProbe)

    .OUTPUTS
        System.Management.Automation.PSCustomObject with winrm, session, boot_time, reboot and error.

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param()
    $winrm = $null
    $failure = $null
    try {
        $winrm = Get-WFWinRMConfiguration
    }
    catch {
        $failure = $_.Exception.Message
    }
    $reboot = $null
    try {
        $reboot = Get-WFPendingReboot
    }
    catch {
        $failure = (@($failure, ("restart state: $($_.Exception.Message)")) | Where-Object { -not [string]::IsNullOrEmpty($_) }) -join '; '
    }
    [PSCustomObject]@{
        winrm     = $winrm
        session   = Get-WFSessionIdentity
        boot_time = Get-WFBootTime
        reboot    = $reboot
        error     = $failure
    }
}
