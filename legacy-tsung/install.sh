# ec2 amazon ami image
sudo yum update -y
sudo yum groupinstall "Development Tools" -y
sudo yum install gcc gcc-c++ glibc-devel make java-1.8.0-openjdk-devel ncurses-devel openssl-devel python-matplotlib gnuplot -y
sudo yum install perl-CPAN -y
sudo yum install 'perl(Template)' 'perl(HTML::Template)'  -y


# install erlang
wget http://erlang.org/download/otp_src_20.1.tar.gz
tar -zxvf otp_src_20.1.tar.gz
rm otp_src_20.1.tar.gz
cd otp_src_20.1
./configure --with-ssl=/usr/include/openssl/
make
sudo make install

# install tsung
wget -c http://tsung.erlang-projects.org/dist/tsung-1.7.0.tar.gz
tar -zxvf tsung-1.7.0.tar.gz
rm tsung-1.7.0.tar.gz
cd ~/tsung-1.7.0
./configure
make
sudo make install
