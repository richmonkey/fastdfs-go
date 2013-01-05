fastdfs-go
==========
## Introduction
implement FastDFS storage service with go.

## Run
1.download FastDFSv2.13 from google code

2.cd FastDFS && patch -Np1 fastdfsv2.13.patch

3.modify CFLAGS in shm.go depend on your FastDFS source directory 

3.go build storage.go shm.go

4.run fdfs_storage && storage

## Todo
implement FaceBook's picture storage Haystack