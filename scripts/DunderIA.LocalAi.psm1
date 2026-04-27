Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Get-DunderIALocalAiProviderConfig {
    [CmdletBinding()]
    param(
        [string]$ConfigPath = (Join-Path $PSScriptRoot 'config\local-ai.providers.json')
    )

    if (-not (Test-Path -LiteralPath $ConfigPath)) {
        throw "Config de providers local AI nao encontrada: $ConfigPath"
    }

    try {
        return Get-Content -LiteralPath $ConfigPath -Raw -Encoding UTF8 | ConvertFrom-Json
    } catch {
        throw "Config de providers local AI invalida em ${ConfigPath}: $($_.Exception.Message)"
    }
}

function Resolve-DunderIALocalAiProfile {
    [CmdletBinding()]
    param(
        [string]$ProviderId = '',
        [string]$ProfileId = '',
        [string]$Model = '',
        [string]$KeepAlive = '',
        [string]$ConfigPath = (Join-Path $PSScriptRoot 'config\local-ai.providers.json')
    )

    $config = Get-DunderIALocalAiProviderConfig -ConfigPath $ConfigPath
    $resolvedProviderId = if ([string]::IsNullOrWhiteSpace($ProviderId)) { [string]$config.defaultProviderId } else { $ProviderId }
    if ([string]::IsNullOrWhiteSpace($resolvedProviderId)) {
        throw 'Nenhum provider default configurado para local AI.'
    }

    $provider = @($config.providers | Where-Object { $_.id -eq $resolvedProviderId } | Select-Object -First 1)[0]
    if ($null -eq $provider) {
        throw "Provider local AI nao encontrado: $resolvedProviderId"
    }

    $resolvedProfileId = if ([string]::IsNullOrWhiteSpace($ProfileId)) {
        if ($provider.PSObject.Properties['defaultProfileId'] -and -not [string]::IsNullOrWhiteSpace([string]$provider.defaultProfileId)) {
            [string]$provider.defaultProfileId
        } else {
            'default'
        }
    } else {
        $ProfileId
    }

    $profile = @($provider.profiles | Where-Object { $_.id -eq $resolvedProfileId } | Select-Object -First 1)[0]
    if ($null -eq $profile) {
        throw "Profile local AI nao encontrado: $resolvedProfileId (provider: $resolvedProviderId)"
    }

    $resolvedModel = if (-not [string]::IsNullOrWhiteSpace($Model)) {
        $Model
    } elseif ($profile.PSObject.Properties['model'] -and -not [string]::IsNullOrWhiteSpace([string]$profile.model)) {
        [string]$profile.model
    } else {
        [string]$provider.defaultModel
    }

    $resolvedKeepAlive = if (-not [string]::IsNullOrWhiteSpace($KeepAlive)) {
        $KeepAlive
    } elseif ($profile.PSObject.Properties['keepAlive'] -and -not [string]::IsNullOrWhiteSpace([string]$profile.keepAlive)) {
        [string]$profile.keepAlive
    } elseif ($provider.PSObject.Properties['keepAlive'] -and -not [string]::IsNullOrWhiteSpace([string]$provider.keepAlive)) {
        [string]$provider.keepAlive
    } else {
        '5m'
    }

    [pscustomobject]@{
        providerId   = [string]$provider.id
        providerKind = [string]$provider.kind
        profileId    = [string]$profile.id
        model        = $resolvedModel
        keepAlive    = $resolvedKeepAlive
        apiUrl       = if ($provider.PSObject.Properties['apiUrl']) { [string]$provider.apiUrl } else { '' }
        exePath      = if ($provider.PSObject.Properties['exePath']) { [string]$provider.exePath } else { '' }
    }
}

function Invoke-DunderIALocalAiGenerate {
    [CmdletBinding()]
    param(
        [string]$ProviderId = '',
        [string]$ProfileId = '',
        [string]$Model = '',
        [Parameter(Mandatory = $true)]
        [string]$Prompt,
        [string]$KeepAlive = '',
        [string]$ConfigPath = (Join-Path $PSScriptRoot 'config\local-ai.providers.json')
    )

    $resolved = Resolve-DunderIALocalAiProfile -ProviderId $ProviderId -ProfileId $ProfileId -Model $Model -KeepAlive $KeepAlive -ConfigPath $ConfigPath
    switch ($resolved.providerKind) {
        'ollama' {
            $body = @{
                model      = $resolved.model
                prompt     = $Prompt
                stream     = $false
                keep_alive = $resolved.keepAlive
            } | ConvertTo-Json -Compress
            try {
                $response = Invoke-RestMethod -Uri $resolved.apiUrl -Method Post -ContentType 'application/json' -Body $body
            } catch {
                throw "Falha ao chamar provider local '$($resolved.providerId)' em $($resolved.apiUrl): $($_.Exception.Message)"
            }

            return [ordered]@{
                providerId      = $resolved.providerId
                profileId       = $resolved.profileId
                providerKind    = $resolved.providerKind
                model           = $response.model
                response        = $response.response
                totalDurationMs = [math]::Round($response.total_duration / 1000000, 2)
                loadDurationMs  = [math]::Round($response.load_duration / 1000000, 2)
                evalCount       = $response.eval_count
                keepAlive       = $resolved.keepAlive
                apiUrl          = $resolved.apiUrl
            }
        }
        default {
            throw "Provider kind nao suportado: $($resolved.providerKind)"
        }
    }
}

Export-ModuleMember -Function Get-DunderIALocalAiProviderConfig, Resolve-DunderIALocalAiProfile, Invoke-DunderIALocalAiGenerate
