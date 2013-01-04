#include <pthread.h>

static key_t shmkey() {
	key_t key;
	key = ftok("/tmp", 0x03);
	return key;
}

ShareMemory *g_shm;
static pthread_mutex_t stat_count_thread_lock;
static int shm_id;
static int init_shm() {
	key_t key;
	void *map;
	key = shmkey();
	if (-1 == key) {
		perror("shmkey");
		return -1;
	}
	shm_id=shmget(key, 0, 0); 
	if (-1 == shm_id) {perror("shmget");return -1;}
	map=shmat(shm_id,NULL,0);
	if ((void*)-1 == map) return -1;
	g_shm = map;
	if (pthread_mutex_init(&stat_count_thread_lock, NULL) != 0) {
		return -1;
	}
	return 0;
}
 
static void update_timestamp(char *ip, int timestamp) {
	int i;
	pthread_mutex_lock(&stat_count_thread_lock);
	for (i = 0; i < FDFS_MAX_SERVERS_EACH_GROUP; i++) {
		FDFSStorageServer *server = g_storage_servers+i;
		if (strcmp(server->server.ip_addr, ip) == 0) {
			printf("ip:%s, %d\n", ip, timestamp);
			server->last_sync_src_timestamp = timestamp;
			g_sync_change_count++;
			break;
		}
	}
	g_storage_stat.last_sync_update = time(NULL);
	g_stat_change_count++;
	pthread_mutex_unlock(&stat_count_thread_lock);
}
