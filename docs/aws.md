# AWS

AWS does not issue JWT token with the IAM service so we will leverage AWS Cognito together with an OpenID Connect identity provider configured in AWS IAM.


> TODO: automate this with Cloudformation

The steps for the configuration are the following:

1. Create an OpenID Connect Identity provider in AWS IAM with the following configurations:
   - The audience should be `sts.amazonaws.com` 
   - Assign a role to the logged in entitites with the Permission Policy `AmazonCognitoPowerUser`. Take note of the Role ARN

> TODO: the permissions could be restricted


2. Configure an Identity Pool in AWS Cognito with the following configurations:
  -  Add the Identity provider configured in the step above 
  -  Enable the Basic Authentication for the pool
  







