// Wails v2 runtime type declarations
declare global {
  interface Window {
    go: {
      main: {
        App: {
          SaveConfig(host: string, port: number, apiKey: string, region: string): Promise<string>;
          GetConfig(): Promise<any>;
          GetDataDirPath(): Promise<string>;
          OpenDataDir(): Promise<string>;
          StartProxy(): Promise<string>;
          StopProxy(): Promise<string>;
          GetStatus(): Promise<any>;
          OneClickStart(): Promise<string>;
          GetProxyLogs(): Promise<string[]>;
          GetCredentialsInfo(): Promise<any>;
          ImportCredentials(path: string): Promise<string>;
          SaveCredentialsRaw(jsonStr: string): Promise<string>;
          ReadCredentialsRaw(): Promise<string>;
          ClearCredentials(): Promise<string>;
          RefreshNow(): Promise<string>;
          ListKeychainSources(): Promise<any[]>;
          UseKeychainSource(source: string): Promise<string>;
          EnsureFactoryApiKey(): Promise<string>;
          ReadFactoryConfig(): Promise<any>;
          WriteFactoryConfig(config: any): Promise<string>;
          ReadDroidSettings(): Promise<any>;
          WriteDroidSettings(settings: any): Promise<string>;
          CheckActivation(): Promise<any>;
          Activate(code: string): Promise<string>;
          Deactivate(): Promise<string>;
          ReadOpenCodeConfig(): Promise<any>;
          WriteOpenCodeConfig(config: any): Promise<string>;
          ReadClaudeCodeSettings(): Promise<any>;
          WriteClaudeCodeSettings(config: any): Promise<string>;
          // Account Management
          GetAccounts(): Promise<any[]>;
          DeleteAccount(id: string): Promise<string>;
          AddAccountBySocial(refreshToken: string, provider: string): Promise<any>;
          AddAccountByIdC(refreshToken: string, clientId: string, clientSecret: string, region: string): Promise<any>;
          SyncAccount(id: string): Promise<any>;
          SwitchAccount(id: string): Promise<string>;
          ImportLocalAccount(): Promise<any>;
          UpdateAccountLabel(id: string, label: string): Promise<any>;
          UpdateAccount(id: string, label: string | null, accessToken: string | null, refreshToken: string | null, clientId: string | null, clientSecret: string | null): Promise<any>;
          ExportAccounts(ids: string[]): Promise<string>;
          ExportAccountsToFile(ids: string[]): Promise<string>;
          BatchDeleteAccounts(ids: string[]): Promise<number>;
          SaveAccountsToFile(filePath: string, content: string): Promise<void>;
          // Tunnel Management
          LoadTunnelConfig(): Promise<any>;
          SaveTunnelConfig(config: any): Promise<string>;
          GetTunnelStatus(): Promise<any>;
          StartTunnel(): Promise<string>;
          StopTunnel(): Promise<string>;
          SetExternalTunnel(url: string): Promise<string>;
          ClearExternalTunnel(): Promise<string>;
        };
      };
    };
  }
}

export {};
