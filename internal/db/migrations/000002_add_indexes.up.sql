-- group_member's only index is the composite unique key (group_id, uid),
-- whose leftmost column is group_id: "which groups is this uid in"
-- (ListRoomsForUser, and the future member-cache invalidation lookups)
-- filters by uid alone and can't use that key, forcing a full table scan.
ALTER TABLE group_member ADD INDEX idx_uid (uid);

-- room_friend's (uid1, uid2) index only serves lookups on uid1 efficiently;
-- ListRoomsForUser's `WHERE uid1 = ? OR uid2 = ?` needs uid2 indexed too so
-- MySQL can satisfy both sides of the OR with an index range scan.
ALTER TABLE room_friend ADD INDEX idx_uid2 (uid2);
