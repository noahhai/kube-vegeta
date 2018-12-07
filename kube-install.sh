# Setting up Kubernetes Cluster with KOPS #

https://medium.com/containermind/how-to-create-a-kubernetes-cluster-on-aws-in-few-minutes-89dda10354f4
# Install kubectl
sudo apt-get install kubectl

# Install awscli
pip install awscli --upgrade --user

# install kops (linux)
curl -LO https://github.com/kubernetes/kops/releases/download/$(curl -s https://api.github.com/repos/kubernetes/kops/releases/latest | grep tag_name | cut -d '"' -f 4)/kops-linux-amd64
chmod +x kops-linux-amd64
sudo mv kops-linux-amd64 /usr/local/bin/kops

# config bucket for kops
bucket_name=bambe-kops-state-store
aws s3api create-bucket \
--bucket ${bucket_name} \
--region us-east-1

# enable versioning for the bucket
aws s3api put-bucket-versioning --bucket ${bucket_name} --versioning-configuration Status=Enabled

export KOPS_CLUSTER_NAME=bambe.k8s.local
export KOPS_STATE_STORE=s3://${bucket_name}

# may be necessary (when running on ubuntu on windows it was)
kops create secret --name bambe.k8s.local sshpublickey admin -i /mnt/c/Users/developer/.ssh/id_rsa.pub

# create configuration for cluster
kops create cluster \
--node-count=2 \
--node-size=t2.small \
--zones=us-east-1a \
--name=${KOPS_CLUSTER_NAME}

# init cluster in aws
kops update cluster --name ${KOPS_CLUSTER_NAME} --yes

# validate cluster
kops validate cluster

# optional - enable dashboard
kubectl apply -f https://raw.githubusercontent.com/kubernetes/dashboard/master/src/deploy/recommended/kubernetes-dashboard.yaml

# get address
kubectl cluster-info

export KUBE_ROOT_ADDRESS=X
dashboard_address=${KUBE_ROOT_ADDRESS}/api/v1/namespaces/kube-system/services/https:kubernetes-dashboard:/proxy/#!/login

# token for login
# for basic auth: 
admin
kops get secrets kube --type secret -oplaintext
# for access token:
kops get secrets admin --type secret -oplaintext







# scale up kops workers
kops edit ig --name= nodes
kops update cluster --yes
kops validate cluster

# scale up loaders
kubectl scale rc vegeta --replicas=3





# cleanup
kubectl delete pod -l run=vegeta --grace-period=0 --force