# AWS

AWS does not issue JWT token with the IAM service so we will leverage AWS Cognito together with an OpenID Connect identity provider configured in AWS IAM.


> TODO: automate this with Cloudformation

The steps for the configuration are the following:

1. Create an OpenID Connect Identity provider in AWS IAM with the following configurations:
   - The audience should be `sts.amazonaws.com` 
   - Assign a role to the logged in entitites with the following Permission Policy:

```json
{
	"Version": "2012-10-17",
	"Statement": [
		{
			"Effect": "Allow",
			"Action": [
				"cognito-identity:*"
			],
			"Resource": "*"
		}
	]
}
```

Take note of the Role ARN
   - Edit the trust relationship as follows:

```json   
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Principal": {
                "Federated": "arn:aws:iam::<your account number>:oidc-provider/<your issuer name>"
            },
            "Action": "sts:AssumeRoleWithWebIdentity",
            "Condition": {
                "StringEquals": {
                    "<your issuer name>:aud": "sts.amazonaws.com",
                    "<your issuer name>:sub": "system:serviceaccount:operator-system:operator-controller-manager"
                }
            }
        }
    ]
}
```




2. Configure an Identity Pool in AWS Cognito with the following configurations:
  -  Add the Identity provider configured in the step above 
  -  Enable the Basic Authentication for the pool
  







