$ErrorActionPreference = "Stop"

$package = if ($env:WUPHF_PLAYWRIGHT_MCP_PACKAGE) {
  $env:WUPHF_PLAYWRIGHT_MCP_PACKAGE
} else {
  "@playwright/mcp@0.0.70"
}

$arguments = @(
  "-y"
  $package
  "--headless"
  "--isolated"
  "--output-mode"
  "stdout"
)

& npx @arguments
