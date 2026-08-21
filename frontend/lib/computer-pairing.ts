function shellQuote(value: string): string {
  return `'${value.replaceAll("'", `'"'"'`)}'`;
}

export function computerPairingCommands(computerId: string, token: string) {
  const server = process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080';
  const installer = process.env.NEXT_PUBLIC_INSTALL_URL ?? 'https://raw.githubusercontent.com/solo-agent/solo/master/scripts/install.sh';
  return {
    installed: `solo daemon connect --server ${shellQuote(server)} --computer-id ${shellQuote(computerId)} --token ${shellQuote(token)} --profile ${shellQuote(computerId)}`,
    fresh: `curl -fsSL ${shellQuote(installer)} | bash -s -- connect --server ${shellQuote(server)} --computer-id ${shellQuote(computerId)} --token ${shellQuote(token)}`,
  };
}
