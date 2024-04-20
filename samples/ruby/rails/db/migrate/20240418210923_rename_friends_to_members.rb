class RenameFriendsToMembers < ActiveRecord::Migration[7.1]
  def change
    rename_table  :friends, :members
  end
end
