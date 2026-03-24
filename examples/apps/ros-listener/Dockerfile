FROM ros:humble

RUN apt-get update && apt-get install -y \
    ros-humble-demo-nodes-cpp \
    ros-humble-rmw-cyclonedds-cpp \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

ENV ROS_DOMAIN_ID=42
ENV RMW_IMPLEMENTATION=rmw_cyclonedds_cpp

CMD ["/bin/bash", "-lc", "source /opt/ros/humble/setup.bash && ros2 run demo_nodes_cpp listener"]
