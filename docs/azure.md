# Azure Entra ID   

1. Create an app registration named `aegis-operator` on Azure Entra ID

2. Create a federated identity credential for the app registration for you kubernetes cluster

3. Give the following API Permissions to the app:
  - `Application.ReadWrite.All`